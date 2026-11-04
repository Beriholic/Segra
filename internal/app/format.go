package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/beriholic/segra/internal/download"
	"github.com/beriholic/segra/locales"
)

func statusLabel(t download.Task) string {
	switch t.Status {
	case download.StatusQueued:
		return locales.Queued()
	case download.StatusDownloading:
		return locales.Downloading()
	case download.StatusPaused:
		return locales.Paused()
	case download.StatusCompleted:
		return locales.CompletedStatus()
	case download.StatusFailed:
		return locales.FailedStatus()
	default:
		return string(t.Status)
	}
}

func statusColor(t download.Task) ui.Color {
	switch t.Status {
	case download.StatusDownloading:
		return colGreen
	case download.StatusCompleted:
		return colGreen
	case download.StatusFailed:
		return colDanger
	case download.StatusPaused:
		return colWarn
	default:
		return colMuted
	}
}

func progressColor(t download.Task) ui.Color {
	if t.Status == download.StatusPaused {
		return colWarn
	}
	if t.Status == download.StatusFailed {
		return colDanger
	}
	return colGreen
}

// progressValue is the row progress bar fill (0..1), or -1 when no bar should
// be drawn. Finished downloads carry no progress information, so they get none.
func progressValue(t download.Task) float64 {
	switch t.Status {
	case download.StatusDownloading, download.StatusPaused, download.StatusFailed:
		if t.Size > 0 {
			return t.Percent()
		}
	}
	return -1
}

func rowMeta(t download.Task) string {
	switch t.Status {
	case download.StatusCompleted:
		return fmt.Sprintf("· %s · %s", formatBytes(t.Size), t.Dir)
	case download.StatusFailed:
		if t.Error != "" {
			return "· " + t.Error
		}
		return ""
	case download.StatusQueued:
		if t.Size > 0 {
			return "· " + sizeLabel(t.Size)
		}
		return ""
	default:
		if t.Size > 0 {
			return fmt.Sprintf("· %s / %s · %d %s", formatBytes(t.Downloaded), formatBytes(t.Size), t.Connections, locales.ConnectionCount())
		}
		return fmt.Sprintf("· %s", formatBytes(t.Downloaded))
	}
}

func splitSpeed(n int64) (string, string) {
	if n <= 0 {
		return "0", "KB/s"
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.0f", float64(n)/1024), "KB/s"
	}
	value := float64(n) / (1024 * 1024)
	if value >= 100 {
		return fmt.Sprintf("%.0f", value), "MB/s"
	}
	return fmt.Sprintf("%.1f", value), "MB/s"
}

func etaLabel(t download.Task) string {
	if t.Speed <= 0 || t.Size <= 0 {
		return ""
	}
	remaining := t.Remaining() / t.Speed
	if remaining <= 0 {
		return ""
	}
	return locales.Remaining() + " " + formatDuration(remaining)
}

func etaValue(t download.Task) string {
	if t.Speed <= 0 || t.Size <= 0 {
		return "—"
	}
	remaining := t.Remaining() / t.Speed
	if remaining <= 0 {
		return "—"
	}
	return formatDuration(remaining)
}

func typeLabel(name string) string {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 || dot == len(name)-1 {
		return "FILE"
	}
	return strings.ToUpper(name[dot+1:])
}

func hostOf(raw string) string {
	rest := raw
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

func activeCount(n int, active bool) int {
	if !active {
		return 0
	}
	return n - n/8
}

func percentLabel(t download.Task) string {
	if t.Size <= 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", t.Percent()*100)
}

func sizeLabel(size int64) string {
	if size < 0 {
		return locales.UnknownSize()
	}
	return formatBytes(size)
}

func formatBytes(n int64) string {
	if n < 0 {
		return locales.UnknownSize()
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			if value >= 100 {
				return fmt.Sprintf("%.0f %s", value, unit)
			}
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PB", value/1024)
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm", seconds/3600, (seconds%3600)/60)
}
