package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// errRestart means the server ignored the requested range or the file
	// changed; the transfer starts over.
	errRestart = errors.New("transfer must restart")
	// errPermanent marks an error that retrying cannot fix.
	errPermanent = errors.New("permanent failure")
)

const (
	// A browser user agent: some CDNs throttle clients that look like bots.
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	probeTimeout     = 30 * time.Second
	stallTimeout     = 10 * time.Second
	maxAttempts      = 4
	copyBufferSize   = 512 << 10
)

var copyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, copyBufferSize)
		return &b
	},
}

type probeResult struct {
	FinalURL    string
	Size        int64
	Ranges      bool
	ETag        string
	LastMod     string
	ContentType string
	Disposition string
	Status      int
}

type httpError struct {
	Status int
	URL    string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d %s (%s)", e.Status, http.StatusText(e.Status), e.URL)
}

func (e *httpError) Unwrap() error { return errPermanent }

func applyHeaders(req *http.Request, in CreateRequest) {
	ua := in.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	if in.Referer != "" {
		req.Header.Set("Referer", in.Referer)
	}
	if in.Cookie != "" {
		req.Header.Set("Cookie", in.Cookie)
	}
}

func probe(ctx context.Context, client *http.Client, in CreateRequest) (*probeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req, in)
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out := &probeResult{
		FinalURL:    resp.Request.URL.String(),
		Size:        -1,
		ETag:        resp.Header.Get("ETag"),
		LastMod:     resp.Header.Get("Last-Modified"),
		ContentType: resp.Header.Get("Content-Type"),
		Disposition: resp.Header.Get("Content-Disposition"),
		Status:      resp.StatusCode,
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range"))
		if !ok {
			return nil, fmt.Errorf("server sent an invalid Content-Range")
		}
		out.Size = total
		out.Ranges = total >= 0
	case http.StatusOK:
		out.Size = resp.ContentLength
		out.Ranges = false
	case http.StatusRequestedRangeNotSatisfiable:
		if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
			out.Size, out.Ranges = total, true
			break
		}
		return nil, &httpError{Status: resp.StatusCode, URL: in.URL}
	default:
		return nil, &httpError{Status: resp.StatusCode, URL: in.URL}
	}
	return out, nil
}

// parseContentRangeTotal reads the total from "bytes 0-0/1234" or
// "bytes */1234". It returns -1 when the total is unknown.
func parseContentRangeTotal(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(strings.ToLower(v), "bytes") {
		return -1, false
	}
	slash := strings.LastIndexByte(v, '/')
	if slash < 0 {
		return -1, false
	}
	total := strings.TrimSpace(v[slash+1:])
	if total == "*" {
		return -1, true
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n < 0 {
		return -1, false
	}
	return n, true
}

func planSegments(size int64, connections int) []segmentState {
	if size <= 0 {
		return []segmentState{{Start: 0, End: -1}}
	}
	if connections < 1 {
		connections = 1
	}
	if int64(connections) > size {
		connections = int(size)
	}
	// Split big files into more chunks than connections so a slow
	// connection only holds up its own chunk; idle workers take the next.
	n := connections
	if size >= 16<<20 {
		if target := int(size / (2 << 20)); target > n {
			n = target
		}
		if limit := connections * 8; n > limit {
			n = limit
		}
	}
	segs := make([]segmentState, 0, n)
	chunk := (size + int64(n) - 1) / int64(n)
	for start := int64(0); start < size; start += chunk {
		end := min(start+chunk-1, size-1)
		segs = append(segs, segmentState{Start: start, End: end})
	}
	return segs
}

// rangeWriter writes a range of the file at an offset and reports its
// progress.
type rangeWriter struct {
	f       *os.File
	off     int64
	written int64
	add     func(int64)
}

func (w *rangeWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.off)
	w.off += int64(n)
	w.written += int64(n)
	if w.add != nil && n > 0 {
		w.add(int64(n))
	}
	return n, err
}

type stallReader struct {
	r     io.Reader
	timer *time.Timer
}

func newStallReader(r io.Reader, d time.Duration, cancel context.CancelFunc) *stallReader {
	return &stallReader{r: r, timer: time.AfterFunc(d, cancel)}
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.timer.Reset(stallTimeout)
	}
	return n, err
}

func (s *stallReader) stop() { s.timer.Stop() }

// fetchRange downloads seg's remaining bytes into f. add reports progress.
func fetchRange(ctx context.Context, client *http.Client, in CreateRequest, m *taskMeta, seg *segmentState, f *os.File, add func(int64)) error {
	length := seg.length()
	attempts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if length >= 0 && seg.Done >= length {
			return nil
		}
		err := fetchRangeOnce(ctx, client, in, m, seg, f, add)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var he *httpError
		if errors.As(err, &he) {
			if he.Status >= 400 && he.Status < 500 && he.Status != http.StatusTooManyRequests {
				return err
			}
		}
		if errors.Is(err, errRestart) || errors.Is(err, errPermanent) {
			return err
		}
		attempts++
		if attempts >= maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempts)):
		}
	}
}

func fetchRangeOnce(ctx context.Context, client *http.Client, in CreateRequest, m *taskMeta, seg *segmentState, f *os.File, add func(int64)) error {
	start := seg.Start + seg.Done
	end := seg.End
	reqCtx, cancelReq := context.WithCancel(ctx)
	defer cancelReq()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, m.URL, nil)
	if err != nil {
		return err
	}
	applyHeaders(req, CreateRequest{
		URL: m.URL, Referer: m.Referer, UserAgent: m.UserAgent, Cookie: m.Cookie,
	})
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	if m.ETag != "" {
		req.Header.Set("If-Range", m.ETag)
	} else if m.LastMod != "" {
		req.Header.Set("If-Range", m.LastMod)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		return errRestart
	case http.StatusRequestedRangeNotSatisfiable:
		return errRestart
	default:
		io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		return &httpError{Status: resp.StatusCode, URL: m.URL}
	}

	if got, want := resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-%d/%d", start, end, m.Size); m.Size >= 0 && got != "" && got != want {
		// A different file or a broken range: start over.
		if !strings.HasPrefix(got, fmt.Sprintf("bytes %d-", start)) {
			return errRestart
		}
	}

	remaining := int64(-1)
	if seg.End >= 0 {
		remaining = seg.End - start + 1
	}
	var body io.Reader = resp.Body
	if remaining >= 0 {
		body = io.LimitReader(resp.Body, remaining)
	}
	sr := newStallReader(body, stallTimeout, cancelReq)
	defer sr.stop()

	w := &rangeWriter{f: f, off: start, add: add}
	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)
	n, err := io.CopyBuffer(w, sr, *bufp)
	if err != nil {
		return err
	}
	if remaining >= 0 && n < remaining {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// fetchSingle streams a response without Range support. It restarts from
// the first byte on failure.
func fetchSingle(ctx context.Context, client *http.Client, in CreateRequest, m *taskMeta, seg *segmentState, f *os.File, add func(int64)) error {
	attempts := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		seg.Done = 0
		if err := f.Truncate(0); err != nil {
			return err
		}
		err := fetchSingleOnce(ctx, client, in, m, seg, f, add)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var he *httpError
		if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 && he.Status != http.StatusTooManyRequests {
			return err
		}
		attempts++
		if attempts >= maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempts)):
		}
	}
}

func fetchSingleOnce(ctx context.Context, client *http.Client, in CreateRequest, m *taskMeta, seg *segmentState, f *os.File, add func(int64)) error {
	reqCtx, cancelReq := context.WithCancel(ctx)
	defer cancelReq()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, m.URL, nil)
	if err != nil {
		return err
	}
	applyHeaders(req, CreateRequest{URL: m.URL, Referer: m.Referer, UserAgent: m.UserAgent, Cookie: m.Cookie})
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
		return &httpError{Status: resp.StatusCode, URL: m.URL}
	}
	if m.Size < 0 && resp.ContentLength > 0 {
		m.Size = resp.ContentLength
	}
	sr := newStallReader(resp.Body, stallTimeout, cancelReq)
	defer sr.stop()
	w := &rangeWriter{f: f, off: 0, add: add}
	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)
	_, err = io.CopyBuffer(w, sr, *bufp)
	if err != nil {
		return err
	}
	return nil
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<min(attempt-1, 5)) * 200 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
