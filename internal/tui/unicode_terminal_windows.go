//go:build windows

package tui

import "golang.org/x/sys/windows"

func terminalUnicodeWarnings() []string {
	input, inputErr := windows.GetConsoleCP()
	output, outputErr := windows.GetConsoleOutputCP()
	// No console (e.g. a pipe/IDE harness): do not guess the stream encoding.
	if inputErr != nil || outputErr != nil || input == 0 || output == 0 {
		return nil
	}
	if warning := consoleCodePageWarning(input, output); warning != "" {
		return []string{warning}
	}
	return nil
}
