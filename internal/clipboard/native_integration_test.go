package clipboard

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// Opt-in: this test uses the real desktop clipboard, not mocked commands.
// It restores plain text only, not rich-text/image clipboard formats.
func TestNativeClipboardUnicodeIntegration(t *testing.T) {
	if os.Getenv("AIBADGER_CLIPBOARD_INTEGRATION") != "1" {
		t.Skip("set AIBADGER_CLIPBOARD_INTEGRATION=1 in an interactive desktop session")
	}
	_, reader, ok := verificationCommands(runtime.GOOS)
	if !ok {
		t.Fatal("native clipboard commands unavailable")
	}
	original, err := runCommand(reader, "")
	if err != nil {
		t.Fatalf("read original clipboard: %v", err)
	}
	t.Cleanup(func() {
		if _, err := CopyVerified(original); err != nil {
			t.Errorf("restore original clipboard text: %v", err)
		}
	})
	for _, text := range []string{
		"UTF-8 test metni\n\nTürkçe karakter testi: çÇ ğĞ ıİ öÖ şŞ üÜ İstanbul, Iğdır, Şanlıurfa, Çanakkale\nDosya: çalışma_öğeleri_2026.go\n Kopyala",
		"\ufeffTürkçe BOM korunmalı",
		"\tİstanbul  \r\n\r\nIğdır\r\n",
		"e\u0301 👩🏽‍💻 🇹🇷\nson satır  ",
		strings.Repeat("çÇ ğĞ ıİ öÖ şŞ üÜ\r\n", 20000),
		"",
	} {
		receipt, err := CopyVerified(text)
		if err != nil || !receipt.Verified || receipt.Bytes != len(text) {
			t.Fatalf("native %s clipboard: receipt=%+v err=%v", runtime.GOOS, receipt, err)
		}
	}
}
