# Unicode input and transfer

Badger uses UTF-8 for CLI/API text and extracted text files. It does not normalize
Unicode, transliterate Turkish letters, or guess a legacy source encoding.
Invalid UTF-8 text sources fail explicitly; source files are not modified.
Binary/asset safety rules still apply before text extraction.

`FILE:` retains source bytes. `PREFIX:` and `NEAR:` compare literal Unicode
text, case-sensitively: `I`, `ı`, `İ`, and `i` remain distinct. File resolution
first uses the supplied path and the operating system's filesystem rules.
Error-path sanitization uses language-neutral Go `EqualFold`, with offsets from
the original string rather than a lowercased copy. It is not Turkish collation.

## TUI editing

Bubble Tea delivers Unicode runes to both textarea widgets. Character movement
and Backspace/Delete operate on extended grapheme clusters, including combining
accents, emoji modifiers, ZWJ sequences and regional-indicator flags. Completion
positions use rune columns, not display-cell widths. Attachment previews truncate
at grapheme boundaries and use terminal-cell width; the original attachment is
unchanged.

Textarea remains the existing Bubble Tea widget. Its normal text-input sanitation
(e.g. tabs in editable fields) is separate from lossless file extraction and
clipboard transfer. No in-app text-selection/highlighting model has been added;
mouse/keyboard selection offered by the terminal must be verified in that
terminal. Fonts and terminal versions can differ in emoji display width.

On Windows, native console input is Unicode. A legacy console code page alone
is not evidence of a TUI layout/input failure, so Badger does not display a
startup warning for it or change the code page or PowerShell preferences.
External tools/pipes may still use legacy encodings; use Windows Terminal and
`chcp 65001` when interoperating with tools that require UTF-8 console encoding.
Clipboard encoding and exact readback verification remain independent.

## Automated tests

Run the focused Unicode/input regressions:

```sh
go test ./internal/tui ./internal/extractor ./pkg/badger -run 'Turkish|Unicode|Grapheme|CompletionCursor|NonUTF8'
go test ./internal/clipboard
```

Real clipboard integration is **opt-in**, requires a desktop session, and invokes
actual OS commands. Mock tests are not evidence of OS clipboard compatibility.
The test temporarily changes the clipboard and restores its plain text only;
rich text, images and other clipboard formats are not restored. Save valuable
clipboard contents before running it.

Windows PowerShell:

```powershell
$env:AIBADGER_CLIPBOARD_INTEGRATION = '1'
go test ./internal/clipboard -run TestNativeClipboardUnicodeIntegration -count=1 -v
Remove-Item Env:AIBADGER_CLIPBOARD_INTEGRATION
```

Linux/macOS:

```sh
AIBADGER_CLIPBOARD_INTEGRATION=1 go test ./internal/clipboard -run TestNativeClipboardUnicodeIntegration -count=1 -v
```

Windows tests use real PowerShell `Set-Clipboard` / `Get-Clipboard -Raw`, explicit
UTF-8 stdin/stdout and STA mode. Windows PowerShell cannot set an empty string
with `Set-Clipboard`; the empty case uses Windows Forms `Clipboard.Clear` and
still requires successful readback. Test cases include Turkish letters, CRLF/LF,
tabs, blank lines, trailing spaces, no final newline, combining accents, emoji,
large UTF-8 text and empty text. Failures retain the file-export fallback.

## Manual terminal matrix

In Windows Terminal (PowerShell and CMD profiles), standalone PowerShell/CMD,
and available Linux/macOS terminals:

1. Record terminal version, font, OS, and `chcp` (Windows).
2. Type the text below one character at a time, then paste it as one block.
3. Move through Turkish letters with Left/Right and delete with Backspace/Delete.
   Repeat with `é`, `👩🏽‍💻`, and `🇹🇷`; a character deletion should remove the
   complete grapheme. Test insertion in the middle of a line as well.
4. Resize the terminal and check wrapping, alignment and cursor placement.
5. Exercise terminal-provided selection and paste the selected text into a
   UTF-8-aware editor; verify that it is unchanged.
6. Submit the goal and copy the prompt. Check its byte integrity using clipboard
   readback; save to a file if verification fails. Verify destination-app paste
   separately: copy-time readback cannot prove what another app later changes.
7. Run the native clipboard integration test from each shell. On Windows, repeat
   with code page 65001 and a legacy code page; verify text integrity in both.

```text
UTF-8 test metni

Türkçe karakter testi: çÇ ğĞ ıİ öÖ şŞ üÜ İstanbul, Iğdır, Şanlıurfa, Çanakkale
Dosya: çalışma_öğeleri_2026.go
 Kopyala
```

Synthetic `tea.KeyMsg` tests do not establish physical keyboard-layout support,
terminal selection behavior, or visual rendering correctness. Native clipboard
tests do not establish Windows Terminal/CMD rendering correctness either.
