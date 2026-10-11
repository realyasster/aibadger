package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestLosslessExportThresholdAndSavedBytes(t *testing.T) {
	m := Model{schemaB: "\tTürkçe  \r\n\r\nson  ", metadata: []protocol.ExtractionMetadata{{Path: "large.go", Status: "Full", RequiresExport: true}}}
	m.cfg.LargePromptByteThreshold = 1000000
	if !m.promptDeliveryIsLarge(codeContextPromptKind) {
		t.Fatal("formatter threshold must expose file delivery even below UI threshold")
	}
	m.state = stateContextReady
	_, cmd, handled := m.handleKeyEnter()
	if !handled || cmd == nil {
		t.Fatal("Enter must use recommended file delivery")
	}
	msg, ok := cmd().(savePromptDoneMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected successful file delivery: %+v", msg)
	}
	defer os.Remove(msg.path)
	view := m.viewContextReady()
	if !strings.Contains(view, "save the complete payload") || !strings.Contains(view, "full: 1") {
		t.Fatalf("missing delivery/integrity report: %s", view)
	}
	path, err := savePromptToTempAt(codeContextPromptKind, m.schemaB, t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != m.schemaB {
		t.Fatalf("export changed bytes: %q %v", got, err)
	}
}
