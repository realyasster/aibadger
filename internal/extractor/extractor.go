package extractor

// This file orchestrates command execution and Prompt 2 safety filtering.

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/imageutil"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

var (
	errPrompt2Excluded = errors.New("excluded from Prompt 2")

	// ErrNoSafePrompt2Files is returned when every requested file is filtered
	// out by Prompt 2 safety rules.
	ErrNoSafePrompt2Files = errors.New("no safe files available for Prompt 2 after excluding binary and sensitive files")
)

// ExtractionError reports a mix of usable extraction results and failures.
// When CanProceed is true, the caller may continue with the successful
// extractions and surface the failures as a warning.
type ExtractionError struct {
	Failures   []string
	Excluded   []string
	CanProceed bool
	Metadata   []protocol.ExtractionMetadata
}

func (e *ExtractionError) Unwrap() error {
	if len(e.Failures) == 0 && len(e.Excluded) > 0 && !e.CanProceed {
		return ErrNoSafePrompt2Files
	}
	return nil
}

func (e *ExtractionError) Error() string {
	var parts []string
	if e.Unwrap() != nil {
		parts = append(parts, ErrNoSafePrompt2Files.Error())
	}
	if len(e.Failures) > 0 {
		parts = append(parts, fmt.Sprintf("extraction failed for %d command(s): %s", len(e.Failures), strings.Join(e.Failures, "; ")))
	}
	if len(e.Excluded) > 0 {
		parts = append(parts, fmt.Sprintf("%d command(s) excluded from Prompt 2 safety rules: %s", len(e.Excluded), strings.Join(e.Excluded, "; ")))
	}
	return strings.Join(parts, "; ")
}

// Extractor handles code extraction from the filesystem.
type Extractor struct {
	ProjectRoot     string
	Topology        *model.ProjectTopology
	ExternalContext []model.ExternalContext
}

type externalCommandResult struct {
	content      string
	fullFile     bool
	matched      bool
	displayPath  string
	sourceSize   int
	sourceSHA256 string
}

// NewExtractor creates a new Extractor instance.
func NewExtractor(root string, t *model.ProjectTopology) *Extractor {
	var external []model.ExternalContext
	if t != nil {
		external = t.ExternalContext
	}
	return &Extractor{ProjectRoot: root, Topology: t, ExternalContext: external}
}

// Extract performs the extraction for all commands in parallel.
func (e *Extractor) Extract(commands []Command) ([]protocol.ExtractionResult, error) {
	fileRequested := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		if cmd.Type == "FILE" {
			fileRequested[cmd.Path] = true
		}
	}
	// Apply the same exact-path precedence before starting work. The first FILE
	// wins even if it fails or is excluded; later duplicates cannot replace it.
	effective := make([]Command, 0, len(commands))
	emittedFile := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		if fileRequested[cmd.Path] {
			if cmd.Type != "FILE" || emittedFile[cmd.Path] {
				continue
			}
			emittedFile[cmd.Path] = true
		}
		effective = append(effective, cmd)
	}
	commands = effective

	var wg sync.WaitGroup
	results := make([]protocol.ExtractionResult, len(commands))
	errs := make([]error, len(commands))

	for i, cmd := range commands {
		wg.Add(1)
		go func(idx int, c Command) {
			defer wg.Done()

			result, err := e.processCommandDetailed(c)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = result
		}(i, cmd)
	}

	wg.Wait()

	extracted := make([]protocol.ExtractionResult, 0, len(commands))
	failures := make([]string, 0)
	excludedFailures := make([]string, 0)
	excluded := 0
	var issueMetadata []protocol.ExtractionMetadata
	for i, result := range results {
		if errs[i] != nil {
			status := "Failed"
			if errors.Is(errs[i], errPrompt2Excluded) {
				status = "Excluded"
			}
			issueMetadata = append(issueMetadata, protocol.ExtractionMetadata{Path: commands[i].Path, Status: status, Reason: errs[i].Error()})
			if errors.Is(errs[i], errPrompt2Excluded) {
				excluded++
				excludedFailures = append(excludedFailures, fmt.Sprintf("%s: excluded from Prompt 2", commands[i].Path))
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %v", commands[i].Path, errs[i]))
			continue
		}
		extracted = append(extracted, result)
	}
	if len(extracted) == 0 && excluded > 0 && len(failures) == 0 {
		return nil, &ExtractionError{Excluded: excludedFailures, Metadata: issueMetadata}
	}
	if len(extracted) > 0 && (len(failures) > 0 || excluded > 0) {
		return extracted, &ExtractionError{
			Failures:   append([]string(nil), failures...),
			Excluded:   append([]string(nil), excludedFailures...),
			CanProceed: true,
			Metadata:   issueMetadata,
		}
	}
	if len(failures) > 0 {
		return extracted, &ExtractionError{
			Metadata: issueMetadata,
			Failures: append([]string(nil), failures...),
			Excluded: append([]string(nil), excludedFailures...),
		}
	}
	return extracted, nil
}

func (e *Extractor) processCommand(cmd Command) (string, string, bool, error) {
	r, err := e.processCommandDetailed(cmd)
	return r.Path, r.Content, r.FullFile, err
}

func (e *Extractor) processCommandDetailed(cmd Command) (protocol.ExtractionResult, error) {
	resolvedPath := e.resolveCommandPath(cmd.Path)
	if resolvedPath != "" {
		r, err := e.processLocalCommand(cmd, resolvedPath)
		r.Path = cmd.Path
		return r, err
	}

	externalResult, err := e.processExternalCommand(cmd)
	if err != nil {
		return protocol.ExtractionResult{}, err
	}
	if externalResult.matched {
		return protocol.ExtractionResult{Path: externalResult.displayPath, Content: externalResult.content, FullFile: externalResult.fullFile, SourceSize: externalResult.sourceSize, SourceSHA256: externalResult.sourceSHA256}, nil
	}

	return protocol.ExtractionResult{}, fmt.Errorf("file not found: %s", cmd.Path)
}

func (e *Extractor) processLocalCommand(cmd Command, resolvedPath string) (protocol.ExtractionResult, error) {
	var result protocol.ExtractionResult
	if promptpolicy.IsSensitivePath(resolvedPath) {
		return result, errPrompt2Excluded
	}
	absolutePath := filepath.Join(e.ProjectRoot, resolvedPath)
	if !isWithinProjectRoot(e.ProjectRoot, absolutePath) {
		return result, fmt.Errorf("file not found: %s", cmd.Path)
	}
	kind := filekind.Classify(absolutePath)
	if kind == model.FileKindBinary {
		return result, errPrompt2Excluded
	}
	if kind == model.FileKindAsset {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return result, err
		}
		result.Content = summarizeBinaryFile(resolvedPath, absolutePath, int(info.Size()), kind)
		result.SourceSize = int(info.Size())
		return result, nil
	}
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return result, fmt.Errorf("read %s: %w", cmd.Path, err)
	}
	if !utf8.Valid(data) {
		return result, fmt.Errorf("invalid UTF-8 in %s; provide a UTF-8 source file (no automatic conversion)", cmd.Path)
	}
	result.SourceSize = len(data)
	result.SourceSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	if cmd.Type == "FILE" {
		result.Content, result.FullFile = string(data), true
		return result, nil
	}
	result.Content, _, err = e.extractBlock(string(data), cmd.Type, cmd.Pattern)
	// Selectors are intentionally partial, even when a small fallback includes
	// all lines. They are not a substitute for an explicit FILE request.
	return result, err
}

func (e *Extractor) processExternalCommand(cmd Command) (externalCommandResult, error) {
	requestPath := cmd.Path
	if len(e.ExternalContext) == 0 {
		return externalCommandResult{}, nil
	}
	resolution := externalcontext.ResolveFile(e.ProjectRoot, e.ExternalContext, requestPath)
	matches := filterExternalMatchesForCommand(cmd, resolution.Matches)
	if len(matches) > 1 {
		return externalCommandResult{matched: true}, ambiguousExternalFileError(requestPath, matches)
	}
	if len(matches) == 0 {
		return externalCommandResult{}, nil
	}
	match := matches[0]
	result := externalCommandResult{matched: true, displayPath: match.DisplayPath}
	if promptpolicy.IsSensitivePath(requestPath) || promptpolicy.IsSensitivePath(match.RelPath) {
		return result, errPrompt2Excluded
	}
	absolutePath := match.AbsPath
	kind := filekind.Classify(absolutePath)
	if kind == model.FileKindBinary {
		return result, errPrompt2Excluded
	}
	if kind == model.FileKindAsset {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return result, fmt.Errorf("file not found: %s", requestPath)
		}
		result.content = summarizeBinaryFile(match.DisplayPath, absolutePath, int(info.Size()), kind)
		result.sourceSize = int(info.Size())
		result.fullFile = false
		return result, nil
	}
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return result, fmt.Errorf("file not found: %s", requestPath)
	}
	if !utf8.Valid(data) {
		return result, fmt.Errorf("invalid UTF-8 in %s; provide a UTF-8 source file (no automatic conversion)", requestPath)
	}
	content := string(data)
	result.sourceSize = len(data)
	result.sourceSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	if cmd.Type == "FILE" {
		result.content = content
		result.fullFile = true
		return result, nil
	}

	extracted, _, err := e.extractBlock(content, cmd.Type, cmd.Pattern)
	if err != nil {
		return result, err
	}
	result.content = extracted
	result.fullFile = false
	return result, nil
}

func filterExternalMatchesForCommand(cmd Command, matches []externalcontext.FileMatch) []externalcontext.FileMatch {
	if cmd.Type == "FILE" {
		return matches
	}
	requestPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(cmd.Path))))
	filtered := make([]externalcontext.FileMatch, 0, len(matches))
	for _, match := range matches {
		if requestPath == match.RelPath || requestPath == match.DisplayPath || requestPath == filepath.ToSlash(match.AbsPath) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func ambiguousExternalFileError(requestPath string, matches []externalcontext.FileMatch) error {
	var sb strings.Builder
	sb.WriteString("Ambiguous file reference: ")
	sb.WriteString(requestPath)
	sb.WriteString("\n\nMatches:")
	for _, match := range matches {
		sb.WriteString("\n- ")
		sb.WriteString(match.DisplayPath)
	}
	sb.WriteString("\n\nUse a more specific path to disambiguate.")
	return errors.New(sb.String())
}

func isWithinProjectRoot(root, absPath string) bool {
	rootClean := filepath.Clean(root)
	absClean := filepath.Clean(absPath)
	return absClean == rootClean || strings.HasPrefix(absClean, rootClean+string(filepath.Separator))
}

var imageAssetExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".ico":  true,
}

func isImageAsset(path string) bool {
	return imageAssetExts[strings.ToLower(filepath.Ext(path))]
}

func summarizeBinaryFile(path string, absolutePath string, size int, kind string) string {
	if kind == model.FileKindAsset && isImageAsset(path) {
		meta := imageutil.GetMetadata(absolutePath)
		if meta != nil {
			lines := []string{
				"Binary file: " + path,
				"Kind: image",
				"Format: " + meta.Format,
				"Size: " + protocol.FormatFileSize(int64(size)),
				fmt.Sprintf("Dimensions: %dx%d", meta.Width, meta.Height),
			}
			if meta.AspectRatio != "" {
				lines = append(lines, "Aspect ratio: "+meta.AspectRatio)
			}
			if meta.HasAlpha {
				lines = append(lines, "Alpha: yes")
			}
			return strings.Join(lines, "\n") + "\n"
		}

		return fmt.Sprintf("Binary file: %s\nKind: asset\nSize: %s\nMetadata: unavailable\n",
			path, protocol.FormatFileSize(int64(size)))
	}

	return fmt.Sprintf("Binary file: %s (%s, kind: %s)\n", path, protocol.FormatFileSize(int64(size)), kind)
}
