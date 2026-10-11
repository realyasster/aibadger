package clipboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

const DocsURL = "https://github.com/PVRLabs/aibadger/blob/main/docs/clipboard.md"

var lookPath = exec.LookPath

type command struct {
	name        string
	args        []string
	pipeExample string
	readback    bool
}

// Receipt records the exact UTF-8 payload and whether readback matched it.
type Receipt struct {
	Bytes    int
	SHA256   string
	Verified bool
}

// Copy succeeds only when the clipboard can be read back byte-for-byte.
// Callers can retain their existing error-to-file-export fallback.
func Copy(text string) error {
	_, err := CopyVerified(text)
	return err
}

func CopyVerified(text string) (Receipt, error) {
	return copyVerified(runtime.GOOS, text)
}

var runCommand = func(c command, input string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var out []byte
	var err error
	if c.readback {
		out, err = cmd.Output()
	} else {
		err = cmd.Run()
	}
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), err
}

func copyVerified(goos, text string) (Receipt, error) {
	receipt := Receipt{Bytes: len(text), SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(text)))}
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return receipt, fmt.Errorf("clipboard cannot preserve invalid UTF-8 or NUL; export to a file")
	}
	writer, reader, ok := verificationCommands(goos)
	if !ok {
		return receipt, fmt.Errorf("clipboard verification unavailable on platform: %s; export to a file", goos)
	}
	if _, err := runCommand(writer, text); err != nil {
		return receipt, fmt.Errorf("clipboard write: %w", err)
	}
	actual, err := runCommand(reader, "")
	if err != nil {
		return receipt, fmt.Errorf("clipboard copied but readback unverified: %w", err)
	}
	if actual != text {
		return receipt, fmt.Errorf("clipboard integrity mismatch: expected %d bytes sha256=%s, got %d bytes sha256=%x; export to a file", receipt.Bytes, receipt.SHA256, len(actual), sha256.Sum256([]byte(actual)))
	}
	receipt.Verified = true
	return receipt, nil
}

func verificationCommands(goos string) (command, command, bool) {
	if goos == "windows" {
		// UTF-8 pipe streams are separate from console code pages. Do not set
		// Console.InputEncoding/OutputEncoding: they may change the parent console.
		writeScript := powershellUTF8Script("$reader = [System.IO.StreamReader]::new([Console]::OpenStandardInput(), $u, $false); $text = $reader.ReadToEnd(); if ($text.Length -eq 0) { Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::Clear() } else { Set-Clipboard -Value $text }")
		readScript := powershellUTF8Script("$writer = [System.IO.StreamWriter]::new([Console]::OpenStandardOutput(), $u); $writer.Write([string](Get-Clipboard -Raw)); $writer.Flush()")
		w, ok := commandIfAvailable("powershell", []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", writeScript}, "")
		r := command{name: "powershell", readback: true, args: []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", readScript}}
		return w, r, ok
	}
	w, ok := nativeCommand(goos)
	var r command
	switch goos {
	case "darwin":
		r = command{name: "pbpaste", readback: true}
	case "linux":
		r = command{name: "xclip", readback: true, args: []string{"-selection", "clipboard", "-o"}}
	default:
		return command{}, command{}, false
	}
	if _, err := lookPath(r.name); err != nil {
		ok = false
	}
	return w, r, ok
}

func powershellUTF8Script(body string) string {
	return "$u = New-Object System.Text.UTF8Encoding($false); $ErrorActionPreference = 'Stop'; try { " + body + " } catch { $writer = [System.IO.StreamWriter]::new([Console]::OpenStandardError(), $u); $writer.Write($_.Exception.Message); $writer.Flush(); exit 1 }"
}

// PipeCommand returns the shell command that reads stdin into the native
// clipboard, when this platform has a supported clipboard tool available.
func PipeCommand() (string, bool) {
	return pipeCommand(runtime.GOOS)
}

// Available reports whether this platform supports verified clipboard copying.
// It deliberately does not expose the command or its installation path.
func Available() bool {
	_, _, ok := verificationCommands(runtime.GOOS)
	return ok
}

func pipeCommand(goos string) (string, bool) {
	clip, ok := nativeCommand(goos)
	if !ok {
		return "", false
	}
	return clip.pipeExample, true
}

func nativeCommand(goos string) (command, bool) {
	switch goos {
	case "darwin":
		return commandIfAvailable("pbcopy", nil, "pbcopy")
	case "linux":
		return commandIfAvailable("xclip", []string{"-selection", "clipboard"}, "xclip -selection clipboard")
	case "windows":
		return commandIfAvailable("clip", nil, "clip")
	default:
		return command{}, false
	}
}

func commandIfAvailable(name string, args []string, pipeExample string) (command, bool) {
	if _, err := lookPath(name); err != nil {
		return command{}, false
	}
	return command{name: name, args: append([]string(nil), args...), pipeExample: pipeExample}, true
}
