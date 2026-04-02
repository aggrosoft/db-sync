package cli

import (
	"bytes"
	"strings"
	"testing"

	"db-sync/internal/schema"
	syncapp "db-sync/internal/sync"
)

func TestRunProgressBarRendersPhaseLine(t *testing.T) {
	buffer := &bytes.Buffer{}
	bar := &runProgressBar{writer: buffer}

	bar.Advance(syncapp.ProgressUpdate{
		Phase:     "scanning delete candidates",
		Completed: 1,
		Total:     4,
		TableID:   schema.TableID{Name: "customer"},
		Scope:     "explicit",
		Detail:    "customer [explicit]",
	})

	output := buffer.String()
	for _, want := range []string{"Phase: scanning delete candidates", "(1/4)", "customer [explicit]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want substring %q", output, want)
		}
	}
}

func TestRunProgressBarKeepsTableBarAfterPhaseLine(t *testing.T) {
	buffer := &bytes.Buffer{}
	bar := &runProgressBar{writer: buffer, enabled: true, total: 2}

	bar.Advance(syncapp.ProgressUpdate{Phase: "syncing table", Completed: 1, Total: 2, Detail: "users [explicit]"})
	bar.Advance(syncapp.ProgressUpdate{Completed: 1, Total: 2, TableID: schema.TableID{Name: "users"}, Scope: "explicit"})

	output := buffer.String()
	if !strings.Contains(output, "Phase: syncing table") {
		t.Fatalf("output = %q, want phase line", output)
	}
	if !strings.Contains(output, "users [explicit]") {
		t.Fatalf("output = %q, want table label", output)
	}
	if !strings.Contains(output, "50%") {
		t.Fatalf("output = %q, want progress percentage", output)
	}
}