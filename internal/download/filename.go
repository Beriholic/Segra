package download

import (
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// MetadataPath is the sidecar that keeps a partial download's segments.
func MetadataPath(path string) string { return path + ".segra.json" }

// PartPath is where a task writes before it renames the file into place.
func (t Task) PartPath() string { return t.Path() + ".part" }

// Filename picks a file name from Content-Disposition, then the URL path,
// then a default.
func Filename(rawURL, disposition string) string {
	if n := filenameFromDisposition(disposition); n != "" {
		return Sanitize(n)
	}
	if u, err := url.Parse(rawURL); err == nil {
		name := filepath.Base(u.Path)
		if unescaped, err := url.PathUnescape(name); err == nil {
			name = unescaped
		}
		if name != "" && name != "/" && name != "." {
			return Sanitize(name)
		}
	}
	return "download"
}

func filenameFromDisposition(cd string) string {
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(params["filename"])
}

// Sanitize keeps a name usable on every platform Segra targets.
func Sanitize(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteRune('_')
		case r < 0x20 || r == 0x7f:
			// drop control characters
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name = strings.Trim(b.String(), " .")
	if name == "" {
		return "download"
	}
	// Windows reserves these device names.
	stem := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		keep := 200 - len(ext)
		for keep > 0 && !utf8Start(name[keep]) {
			keep--
		}
		name = name[:keep] + ext
	}
	return name
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// UniqueName returns a name in dir that no file of the app uses, adding
// " (2)", " (3)" before the extension as needed.
func UniqueName(dir, name string) string {
	if !nameTaken(dir, name) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i < 10000; i++ {
		cand := base + " (" + itoa(i) + ")" + ext
		if !nameTaken(dir, cand) {
			return cand
		}
	}
	return base + " (" + itoa(os.Getpid()) + ")" + ext
}

func nameTaken(dir, name string) bool {
	p := filepath.Join(dir, name)
	for _, q := range []string{p, p + ".part", MetadataPath(p + ".part")} {
		if _, err := os.Stat(q); err == nil {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
