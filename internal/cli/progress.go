package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"db-sync/internal/schema"
	syncapp "db-sync/internal/sync"

	"golang.org/x/term"
)

type runProgressBar struct {
	writer  io.Writer
	enabled bool
	total   int
	dryRun  bool
	mutex   sync.Mutex
	active  bool
	lastLen int
}

func newRunProgressBar(writer io.Writer, total int, dryRun bool) *runProgressBar {
	return &runProgressBar{
		writer:  writer,
		enabled: total > 0 && isTTYWriter(writer),
		total:   total,
		dryRun:  dryRun,
	}
}

func (bar *runProgressBar) Advance(update syncapp.ProgressUpdate) {
	if update.Phase != "" {
		bar.renderPhase(update)
		return
	}
	if !bar.enabled {
		return
	}
	bar.mutex.Lock()
	defer bar.mutex.Unlock()

	completed := update.Completed
	if completed < 0 {
		completed = 0
	}
	if completed > bar.total {
		completed = bar.total
	}
	mode := "run"
	if bar.dryRun {
		mode = "dry-run"
	}
	width := 24
	filled := 0
	if bar.total > 0 {
		filled = completed * width / bar.total
	}
	if filled > width {
		filled = width
	}
	label := displayProgressTable(update.TableID, update.Scope)
	line := fmt.Sprintf("[%s%s] %3d%% %s (%d/%d)", strings.Repeat("#", filled), strings.Repeat("-", width-filled), percent(completed, bar.total), label, completed, bar.total)
	if mode != "run" {
		line += " " + mode
	}
	line = truncateProgressLine(line, progressLineLimit(bar.writer))
	padding := ""
	if len(line) < bar.lastLen {
		padding = strings.Repeat(" ", bar.lastLen-len(line))
	}
	_, _ = fmt.Fprintf(bar.writer, "\r%s%s", line, padding)
	bar.lastLen = len(line)
	bar.active = true
	if completed == bar.total {
		_, _ = fmt.Fprint(bar.writer, "\n")
		bar.active = false
		bar.lastLen = 0
	}
}

func (bar *runProgressBar) renderPhase(update syncapp.ProgressUpdate) {
	bar.mutex.Lock()
	defer bar.mutex.Unlock()
	if bar.active {
		_, _ = fmt.Fprint(bar.writer, "\n")
		bar.active = false
		bar.lastLen = 0
	}
	line := fmt.Sprintf("Phase: %s", update.Phase)
	if update.Completed > 0 && update.Total > 0 {
		line += fmt.Sprintf(" (%d/%d)", update.Completed, update.Total)
	}
	if update.Detail != "" {
		line += " - " + update.Detail
	}
	_, _ = fmt.Fprintln(bar.writer, truncateProgressLine(line, progressLineLimit(bar.writer)))
}

func (bar *runProgressBar) Finish() {
	if !bar.enabled {
		return
	}
	bar.mutex.Lock()
	defer bar.mutex.Unlock()
	if !bar.active {
		return
	}
	_, _ = fmt.Fprintf(bar.writer, "\r%s\r", strings.Repeat(" ", bar.lastLen))
	_, _ = fmt.Fprint(bar.writer, "\n")
	bar.active = false
	bar.lastLen = 0
}

func displayProgressTable(tableID schema.TableID, scope string) string {
	name := displayTableID(tableID)
	if scope == "" {
		return name
	}
	return fmt.Sprintf("%s [%s]", name, scope)
}

func percent(completed int, total int) int {
	if total <= 0 {
		return 100
	}
	return completed * 100 / total
}

func truncateProgressLine(line string, limit int) string {
	if limit <= 0 || len(line) <= limit {
		return line
	}
	if limit <= 3 {
		return line[:limit]
	}
	return line[:limit-3] + "..."
}

func progressLineLimit(writer io.Writer) int {
	type fdWriter interface {
		Fd() uintptr
	}
	fd, ok := writer.(fdWriter)
	if !ok {
		return 120
	}
	width, _, err := term.GetSize(int(fd.Fd()))
	if err != nil || width <= 0 {
		return 120
	}
	if width < 20 {
		return 20
	}
	return width - 1
}
