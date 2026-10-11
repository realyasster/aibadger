package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestUnicodeSelectorsAreExactWithoutNormalization(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("çalışma", "öğeleri_2026.go")
	source := "package main\n\nfunc İğdır() {\n\tprintln(`şŞ  çÇ e\u0301 👩🏽‍💻`)\n}\n\nfunc Isparta() {}"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewExtractor(root, nil)
	for _, c := range []Command{{Type: "FILE", Path: path}, {Type: "PREFIX", Path: path, Pattern: "func İğdır"}, {Type: "NEAR", Path: path, Pattern: "şŞ"}, {Type: "NEAR", Path: path, Pattern: "e\u0301"}} {
		results, err := e.Extract([]Command{c})
		if err != nil || len(results) != 1 {
			t.Fatalf("%+v: results=%v err=%v", c, results, err)
		}
		if c.Type == "FILE" && results[0].Content != source {
			t.Fatal("file bytes changed")
		}
	}
	for _, pattern := range []string{"iğdır", "ısparta", "é"} {
		results, err := e.Extract([]Command{{Type: "NEAR", Path: path, Pattern: pattern}})
		if err == nil || len(results) != 0 {
			t.Fatalf("must not fold/normalize %q: %v %v", pattern, results, err)
		}
	}
}

func TestRejectInvalidUTF8LocalAndExternalSource(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	for _, dir := range []string{root, external} {
		if err := os.WriteFile(filepath.Join(dir, "kodlama.go"), []byte("package main\n// \xff\xfe"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []*Extractor{NewExtractor(root, nil), {ProjectRoot: t.TempDir(), ExternalContext: []model.ExternalContext{{Path: "../çalışma", AbsPath: external}}}} {
		for _, kind := range []string{"FILE", "PREFIX", "NEAR"} {
			results, err := e.Extract([]Command{{Type: kind, Path: "kodlama.go", Pattern: "package"}})
			if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") || len(results) != 0 {
				t.Fatalf("%s: results=%v err=%v", kind, results, err)
			}
		}
	}
}
