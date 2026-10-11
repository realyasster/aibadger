package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileExtractionPreservesLanguageWhitespace(t *testing.T) {
	root := t.TempDir()
	cases := map[string]string{
		"large.go":               "package main\r\n\r\nfunc Long() {\r\n" + strings.Repeat("\tprintln(\"Türkçe\")  \r\n", 5000) + "}",
		"indent.py":              "def f():\n    x = 'İstanbul'  \n\n    if x:\n\tprint(x)\n\n# son  ",
		"literal.js":             "const value = `\r\n\t${'ğ'}\r\n\r\n  `;  ",
		"empty.txt":              "",
		"binary-description.txt": "Binary file: this is plain text\n",
	}
	e := NewExtractor(root, nil)
	for name, source := range cases {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		results, err := e.Extract([]Command{{Type: "FILE", Path: name}})
		if err != nil || len(results) != 1 || results[0].Content != source || !results[0].FullFile {
			t.Fatalf("%s: results=%v err=%v", name, results, err)
		}
	}
}
