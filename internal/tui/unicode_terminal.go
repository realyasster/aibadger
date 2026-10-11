package tui

import "fmt"

func consoleCodePageWarning(input, output uint32) string {
	if input == 65001 && output == 65001 {
		return ""
	}
	return fmt.Sprintf("Console code pages are not UTF-8 (input %d, output %d). Native console input uses Unicode, but external tools or pipes may corrupt non-ASCII text. Use Windows Terminal and run chcp 65001 before launching Badger. Clipboard encoding is checked separately.", input, output)
}
