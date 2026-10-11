# Clipboard Dependencies

Badger copies one UTF-8 payload, computes its byte length and SHA-256 digest,
and reads the clipboard back. Success requires an exact byte-for-byte match:
line endings and whitespace are not normalized. A write failure, unreadable
clipboard, or mismatch triggers the existing lossless file-export fallback.
Invalid UTF-8 and embedded NUL bytes must be exported to a file.

Readback verifies the clipboard at copy time, not what a destination application
will later paste or modify. Below are the required tools per platform.

## Linux

`xclip` with the `-selection clipboard` flag.

```bash
# Debian / Ubuntu / Linux Mint
sudo apt install xclip

# Fedora
sudo dnf install xclip

# Arch Linux
sudo pacman -S xclip

# openSUSE
sudo zypper install xclip
```
> [!IMPORTANT]
> `xclip` is not available on every Linux distribution by default. If it is
> missing, you will see a "clipboard copy failed" error. Install it using one
> of the commands above.

## macOS

`pbcopy` and `pbpaste` — built-in, no installation required.

## Windows

Windows PowerShell (`powershell`) with `Set-Clipboard` and `Get-Clipboard`,
using explicitly configured UTF-8 stdin/stdout. Badger's verified copy does not
use `clip.exe`, avoiding its code-page-dependent Unicode conversion.

The shell pipe suggestion may still use `clip`; manual shell copying is not
verified by Badger.

For Turkish input, grapheme editing, terminal warnings and real clipboard
integration test instructions, see [Unicode input and transfer](unicode.md).

## Large source context

`FILE:` text is retained exactly as read, including CRLF/LF, tabs, blank lines,
trailing spaces, and a missing final newline. Prompt delimiters surround the
source; they are not part of its byte count or digest.

`MaxContextFileBytes` and `MaxPromptTwoBytes` are now delivery thresholds, not
truncation limits. When exceeded, all extracted bytes remain in the output and
the TUI offers file delivery even below its normal large-prompt threshold.
Choose **Save to temp file** or **Save to Downloads** and attach the complete
file. Non-interactive callers can redirect stdout to a file. This implementation
uses complete file export rather than a multipart clipboard workflow.

Integrity metadata distinguishes full files, selected spans, failed reads and
safety exclusions. `PREFIX:`/`NEAR:` are partial selections, not full-file
requests, and a missing pattern is an error. Binary/asset summaries do not count
as full original-file extraction. Security filters and file-writing whitespace
modes remain unchanged.
