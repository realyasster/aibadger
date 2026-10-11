package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"
)

const turkishInput = "Türkçe karakter testi: çÇ ğĞ ıİ öÖ şŞ üÜ İstanbul, Iğdır, Şanlıurfa, Çanakkale Dosya: çalışma_öğeleri_2026.go"

func unicodeModel(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.SkipOnboarding = true
	cfg.SettingsPath = filepath.Join(root, "settings.json")
	return NewModel(root, cfg)
}

func TestTurkishTypingAndPastePreserveRunes(t *testing.T) {
	for _, rapid := range []bool{false, true} {
		m := unicodeModel(t)
		for _, r := range turkishInput {
			if !rapid {
				m.goalInputLastRuneAt = time.Time{}
			}
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = next.(Model)
		}
		m.finishGoalPasteCapture()
		if m.goalInput.Value() != turkishInput {
			t.Fatalf("rapid=%v: %q", rapid, m.goalInput.Value())
		}
		if !utf8.ValidString(m.View()) {
			t.Fatal("rendered invalid UTF-8")
		}
	}
	m := unicodeModel(t)
	text := "UTF-8 test metni\n\n" + turkishInput + "\n Kopyala"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	m = next.(Model)
	if m.goalInput.Value() != text {
		t.Fatalf("paste changed: %q", m.goalInput.Value())
	}
}

func TestTurkishEditingInMiddleDoesNotBecomeAppendPaste(t *testing.T) {
	m := unicodeModel(t)
	m.setGoalInputValue("çş")
	m.goalInput.SetCursor(1)
	for _, r := range "İğ" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	m.finishGoalPasteCapture()
	if m.goalInput.Value() != "çİğş" {
		t.Fatalf("middle typing moved: %q", m.goalInput.Value())
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(Model)
	if m.goalInput.Value() != "çİş" {
		t.Fatalf("backspace: %q", m.goalInput.Value())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if next.(Model).goalInput.Value() != "çİ" {
		t.Fatalf("delete: %q", next.(Model).goalInput.Value())
	}
}

func TestEditorGraphemeMovementAndDeletion(t *testing.T) {
	for _, cluster := range []string{"ş", "e\u0301", "👩🏽‍💻", "🇹🇷"} {
		for _, field := range []string{"goal", "paste"} {
			m := unicodeModel(t)
			editor := &m.goalInput
			if field == "paste" {
				editor = &m.paste
				editor.Focus()
			}
			editor.SetValue("ç" + cluster + "ş\nğ")
			editor.CursorStart() // SetValue leaves the cursor on the last line.
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyCtrlHome})
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyRight})
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyRight})
			col := editor.LineInfo().StartColumn + editor.LineInfo().ColumnOffset
			if col != 1+utf8.RuneCountInString(cluster) {
				t.Fatalf("%s %q right cursor=%d", field, cluster, col)
			}
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyLeft})
			if info := editor.LineInfo(); info.StartColumn+info.ColumnOffset != 1 {
				t.Fatalf("%s %q left split the cluster", field, cluster)
			}
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyRight})
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyBackspace})
			if editor.Value() != "çş\nğ" {
				t.Fatalf("%s %q backspace=%q", field, cluster, editor.Value())
			}
			editor.SetValue("ç" + cluster + "\nğ")
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyCtrlHome})
			editor.SetCursor(1)
			*editor, _ = updateUnicodeTextarea(*editor, tea.KeyMsg{Type: tea.KeyDelete})
			if editor.Value() != "ç\nğ" {
				t.Fatalf("%s %q delete/line merge=%q", field, cluster, editor.Value())
			}
		}
	}
}

func TestPasteWidgetTurkishRunes(t *testing.T) {
	m := unicodeModel(t)
	m.paste.Focus()
	for _, r := range turkishInput {
		m.paste, _ = updateUnicodeTextarea(m.paste, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.paste.Value() != turkishInput {
		t.Fatalf("typed paste widget changed: %q", m.paste.Value())
	}
	m.paste.SetValue("")
	m.paste, _ = updateUnicodeTextarea(m.paste, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(turkishInput), Paste: true})
	if m.paste.Value() != turkishInput {
		t.Fatalf("pasted widget changed: %q", m.paste.Value())
	}
}

func TestCompletionCursorUsesRuneColumnsNotCellWidths(t *testing.T) {
	for _, prefix := range []string{"ç", "界", "e\u0301", "👩🏽‍💻"} {
		m := unicodeModel(t)
		m.setGoalInputValue(prefix + " @çalışma.go suffix")
		m.goalInput.SetCursor(utf8.RuneCountInString(prefix + " @çalışma.go"))
		if got := m.goalInputCursorByteIndex(); got != len(prefix+" @çalışma.go") {
			t.Fatalf("%q cursor byte=%d", prefix, got)
		}
	}
}

func TestUnicodeAttachmentInsertionAndPreview(t *testing.T) {
	if got := insertedText("ş", "ış"); got != "ı" {
		t.Fatalf("shared UTF-8 prefix split a rune: %q", got)
	}
	if got := insertedText("ğş", "ğİş"); got != "İ" {
		t.Fatalf("middle insert: %q", got)
	}
	for _, cluster := range []string{"e\u0301", "👩🏽‍💻", "🇹🇷"} {
		text := strings.Repeat(cluster, 10)
		if clipped, truncated := firstValidUTF8Bytes(text, len(cluster)-1); !truncated || clipped != "" {
			t.Fatalf("byte preview split grapheme: %q", clipped)
		}
		got := truncateGoalAttachmentRow(text, 4)
		if !utf8.ValidString(got) || uniseg.StringWidth(got) > 4 {
			t.Fatalf("invalid preview %q", got)
		}
		prefix := strings.TrimSuffix(got, "…")
		if prefix != strings.Repeat(cluster, uniseg.GraphemeClusterCount(prefix)) {
			t.Fatalf("split grapheme: %q", got)
		}
	}
}

func TestUnicodeEditorWrapCountsTerminalCells(t *testing.T) {
	for _, cluster := range []string{"ş", "e\u0301", "👩🏽‍💻", "🇹🇷"} {
		text := strings.Repeat(cluster, 10)
		width := uniseg.StringWidth(text)
		want := (width + 4) / 5
		if got := countEditorVisualLines(text, 5); got != want {
			t.Fatalf("%q rows=%d want=%d", cluster, got, want)
		}
	}
}
