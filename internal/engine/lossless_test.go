package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/model"
)

func TestTwelveFileRequestsAreLossless(t *testing.T) {
	root := t.TempDir()
	eng := FromTopology(root, &model.ProjectTopology{})
	// Both thresholds must trigger export rather than modify extraction.
	eng.formatter.MaxContextFileBytes = 32
	eng.formatter.MaxPromptTwoBytes = 100
	var commands []extractor.Command
	var sources []string
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("file%d.txt", i)
		content := fmt.Sprintf("file %d\r\n\tTürkçe  \r\n\r\n", i) + strings.Repeat("const x = `\n  ${value}\n`;\n", 4000) + "son satır  "
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, extractor.Command{Type: "FILE", Path: name})
		sources = append(sources, content)
	}
	prompt, metadata, count, failed, excluded, err := eng.GenerateContextDetailed("test", commands)
	if err != nil || count != 12 || len(metadata) != 12 || len(failed)+len(excluded) != 0 {
		t.Fatalf("count=%d metadata=%d err=%v", count, len(metadata), err)
	}
	for i, meta := range metadata {
		want := fmt.Sprintf("--- File: %s (Full File) ---\n", commands[i].Path) + sources[i] + "\n--- End File ---"
		if !strings.Contains(prompt, want) {
			t.Fatalf("file %d changed", i)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(sources[i])))
		if meta.Status != "Full" || meta.Truncated || meta.Dropped || !meta.RequiresExport || meta.OriginalSize != len(sources[i]) || meta.TransferredSize != len(sources[i]) || meta.SHA256 != hash || meta.SourceSHA256 != hash {
			t.Fatalf("bad metadata: %+v", meta)
		}
	}
}

func TestLongSelectorBlockAndFailureReport(t *testing.T) {
	root := t.TempDir()
	source := "func Long() {\n" + strings.Repeat("\tprintln(`ş { }`)\n", 100) + "}\n"
	if err := os.WriteFile(filepath.Join(root, "long.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	eng := FromTopology(root, &model.ProjectTopology{})
	prompt, meta, count, failed, _, err := eng.GenerateContextDetailed("test", []extractor.Command{{Type: "PREFIX", Path: "long.go", Pattern: "func Long"}, {Type: "NEAR", Path: "long.go", Pattern: "missing"}})
	if err != nil || count != 1 || len(failed) != 1 || len(meta) != 2 {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}
	if !strings.Contains(prompt, source) || meta[0].Status != "Partial" || meta[1].Status != "Failed" {
		t.Fatalf("incomplete or misleading output: %+v", meta)
	}
}
