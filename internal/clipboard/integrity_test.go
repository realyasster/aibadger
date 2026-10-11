package clipboard

import (
	"errors"
	"strings"
	"testing"
)

func TestClipboardIntegrityAcrossPlatforms(t *testing.T) {
	oldRun, oldLook := runCommand, lookPath
	t.Cleanup(func() { runCommand, lookPath = oldRun, oldLook })
	lookPath = func(name string) (string, error) { return name, nil }
	for _, platform := range []string{"windows", "linux", "darwin"} {
		for _, text := range []string{"", "\tTürkçe: ıİşŞğĞüÜöÖçÇ  \r\n\r\nson", "a\n\nb\n", "const x = `\n  ${value}\n`;", strings.Repeat("ş", 100000)} {
			stored := ""
			calls := 0
			runCommand = func(c command, input string) (string, error) {
				calls++
				if calls == 1 {
					stored = input
					return "", nil
				}
				return stored, nil
			}
			receipt, err := copyVerified(platform, text)
			if err != nil || !receipt.Verified || receipt.Bytes != len(text) || len(receipt.SHA256) != 64 {
				t.Fatalf("%s: receipt=%+v err=%v", platform, receipt, err)
			}
		}
	}
}

func TestClipboardMismatchAndUnreadableAreNotSuccess(t *testing.T) {
	oldRun, oldLook := runCommand, lookPath
	t.Cleanup(func() { runCommand, lookPath = oldRun, oldLook })
	lookPath = func(name string) (string, error) { return name, nil }
	for _, readErr := range []error{nil, errors.New("unavailable")} {
		calls := 0
		runCommand = func(c command, input string) (string, error) {
			calls++
			if calls == 1 {
				return "", nil
			}
			return "changed\r\n", readErr
		}
		receipt, err := copyVerified("linux", "original\n")
		if err == nil || receipt.Verified {
			t.Fatalf("must not report verified: %+v %v", receipt, err)
		}
	}
	for _, text := range []string{"a\x00b", string([]byte{0xff})} {
		if receipt, err := copyVerified("windows", text); err == nil || receipt.Verified {
			t.Fatal("unsupported clipboard bytes must require export")
		}
	}
}
