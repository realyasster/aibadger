package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

// Bubble Tea delivers runes; textarea cursor columns are rune indices, not
// terminal cell widths. Keep the widget's native editing/view machinery while
// making horizontal movement and character deletion atomic for graphemes.
func updateUnicodeTextarea(editor textarea.Model, msg tea.KeyMsg) (textarea.Model, tea.Cmd) {
	if !editor.Focused() {
		return editor.Update(msg)
	}
	lines := strings.Split(editor.Value(), "\n")
	line := lines[editor.Line()]
	info := editor.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	previous, next := 0, utf8.RuneCountInString(line)
	boundary := 0
	clusters := uniseg.NewGraphemes(line)
	for clusters.Next() {
		end := boundary + utf8.RuneCountInString(clusters.Str())
		if boundary < col {
			previous = boundary
		}
		if end > col {
			next = end
			break
		}
		boundary = end
	}
	var steps int
	switch {
	case key.Matches(msg, editor.KeyMap.CharacterBackward) && col > 0:
		steps = col - previous
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case key.Matches(msg, editor.KeyMap.CharacterForward) && col < utf8.RuneCountInString(line):
		steps = next - col
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case key.Matches(msg, editor.KeyMap.DeleteCharacterBackward) && col > 0:
		steps = col - previous
		if boundary < col && col < next {
			editor.SetCursor(next)
			steps = next - boundary
		}
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case key.Matches(msg, editor.KeyMap.DeleteCharacterForward) && col < utf8.RuneCountInString(line):
		// Native forward-delete can merge the following line when deleting the
		// last rune. Delete backwards from the cluster end instead.
		start := col
		if previous < col && boundary < col {
			start = boundary
		}
		editor.SetCursor(next)
		steps = next - start
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	}
	if steps == 0 {
		updated, cmd := editor.Update(msg)
		if key.Matches(msg, editor.KeyMap.LinePrevious, editor.KeyMap.LineNext, editor.KeyMap.WordBackward, editor.KeyMap.WordForward) {
			snapTextareaCursor(&updated)
		}
		return updated, cmd
	}
	var cmds []tea.Cmd
	for i := 0; i < steps; i++ {
		var cmd tea.Cmd
		editor, cmd = editor.Update(msg)
		cmds = append(cmds, cmd)
	}
	return editor, tea.Batch(cmds...)
}

func snapTextareaCursor(editor *textarea.Model) {
	line := strings.Split(editor.Value(), "\n")[editor.Line()]
	info := editor.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	boundary := 0
	clusters := uniseg.NewGraphemes(line)
	for clusters.Next() {
		end := boundary + utf8.RuneCountInString(clusters.Str())
		if boundary < col && col < end {
			editor.SetCursor(boundary)
			return
		}
		boundary = end
	}
}
