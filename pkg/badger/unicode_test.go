package badger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReplacePathFoldPreservesOriginalUnicodeOffsets(t *testing.T) {
	cases := []struct{ message, path, replacement, want string }{
		{"İstanbul: /çalışma/İğdır failed", "/çalışma/İğdır", "<repository>", "İstanbul: <repository> failed"},
		{"K: /K/ŞANLIURFA failed", "/k/şanliurfa", "<repository>", "K: <repository> failed"},
		{"/K/ş /K/Ş", "/k/ş", "X", "X X"},
		{"I ı İ i", "I", "X", "X ı İ X"},
		{"same/path same/path", "same/path", "same/path!", "same/path! same/path!"},
		{"çalışma", "", "X", "çalışma"},
		{"e\u0301 é", "é", "X", "e\u0301 X"},
	}
	for _, c := range cases {
		if got := replacePathFold(c.message, c.path, c.replacement); got != c.want || !utf8.ValidString(got) {
			t.Fatalf("%q: got=%q want=%q", c.message, got, c.want)
		}
	}
}

func TestAPIUnicodePathsAndSourceBytes(t *testing.T) {
	root := writeAPITestProject(t)
	name := filepath.Join("çalışma_öğeleri", "Iğdır_İstanbul_Şanlıurfa.go")
	source := "package main\r\n\r\n// çÇ ğĞ ıİ öÖ şŞ üÜ e\u0301 👩🏽‍💻\r\nfunc Çalış() {\r\n\tprintln(`Türkçe  `)\r\n}"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	goal := writeAPITestInput(t, "türkçe-goal.txt", "İstanbul, Iğdır, Şanlıurfa, Çanakkale")
	for _, selector := range []string{"FILE:" + name, "PREFIX:" + name + "#func Çalış", "NEAR:" + name + "#Türkçe"} {
		input := writeAPITestInput(t, "seçimler.txt", selector)
		var out, diagnostics bytes.Buffer
		err := RunAPI(Config{Root: root}, APIOptions{Operation: "extract", InputPath: input, GoalFilePath: goal, Stdout: &out, Stderr: &diagnostics})
		if err != nil || !utf8.Valid(out.Bytes()) || !strings.Contains(out.String(), "Türkçe") || !strings.Contains(out.String(), name) {
			t.Fatalf("%s: err=%v output=%s", selector, err, out.String())
		}
		if strings.HasPrefix(selector, "FILE:") && !strings.Contains(out.String(), source) {
			t.Fatal("source bytes changed")
		}
	}
	bad := filepath.Join(root, "kodlama.go")
	invalid := []byte{'/', '/', ' ', 0xfe, 0xff}
	if err := os.WriteFile(bad, invalid, 0600); err != nil {
		t.Fatal(err)
	}
	input := writeAPITestInput(t, "invalid-selectors.txt", "FILE:kodlama.go")
	var out bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "extract", InputPath: input, GoalFilePath: goal, Stdout: &out, Stderr: &out})
	if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("must reject non-UTF-8 sources without conversion: %v", err)
	}
	got, readErr := os.ReadFile(bad)
	if readErr != nil || !bytes.Equal(got, invalid) {
		t.Fatalf("invalid source changed: %v", readErr)
	}
}
