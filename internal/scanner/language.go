package scanner

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

// Source language is independent of file kind, manifest ownership and display
// summary caps. Ambiguous headers (.h/.vh) do not cast language votes.
var sourceExtensionLanguages = map[string]string{
	".go": "Go", ".java": "Java", ".py": "Python",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".mts": "TypeScript", ".cts": "TypeScript",
	".cpp": "C++", ".cc": "C++", ".cxx": "C++", ".c": "C",
	".rs": "Rust", ".rb": "Ruby", ".php": "PHP", ".cs": "C#",
	".kt": "Kotlin", ".kts": "Kotlin", ".swift": "Swift",
	".ads": "Ada", ".adb": "Ada", ".ada": "Ada",
	".cbl": "COBOL", ".cob": "COBOL", ".cobol": "COBOL", ".ccp": "COBOL", ".cpy": "COBOL",
	".jcl": "JCL", ".sv": "SystemVerilog", ".svh": "SystemVerilog", ".vhd": "VHDL", ".vhdl": "VHDL",
	".f77": "Fortran", ".f90": "Fortran", ".f95": "Fortran", ".f03": "Fortran", ".f08": "Fortran", ".fpp": "Fortran", ".ftn": "Fortran",
	".pli": "PL/I", ".pl1": "PL/I", ".rpgle": "RPG", ".sqlrpgle": "RPG", ".rpgleinc": "RPG", ".sqlrpg": "RPG",
	".clle": "IBM CL", ".clp": "IBM CL", ".clp38": "IBM CL", ".abap": "ABAP",
}

func sourceExtensionLanguage(path string) string {
	return sourceExtensionLanguages[strings.ToLower(filepath.Ext(path))]
}

func sourceFileLanguage(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".vue") {
		return vueSourceLanguage(path)
	}
	return sourceExtensionLanguage(path)
}

var vueScriptTag = regexp.MustCompile(`(?i)<script\b([^>]*)>`)
var vueScriptClose = regexp.MustCompile(`(?i)</script\s*>`)
var vueScriptAttribute = regexp.MustCompile(`(?i)(?:^|\s)([a-z_:][a-z0-9_:.-]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+)))?`)
var vueComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func vueSourceLanguage(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return ""
	}
	text := vueComment.ReplaceAllString(string(data), "")
	language := "Vue" // Template-only components are not guessed to be JS or TS.
	for offset := 0; offset < len(text); {
		tag := vueScriptTag.FindStringSubmatchIndex(text[offset:])
		if tag == nil {
			break
		}
		attributes := text[offset+tag[2] : offset+tag[3]]
		headerEnd := offset + tag[1]
		// Do not mistake script-tag strings inside JS/TS code for SFC blocks.
		if close := vueScriptClose.FindStringIndex(text[headerEnd:]); close != nil {
			offset = headerEnd + close[1]
		} else {
			offset = len(text)
		}
		lang := ""
		for _, attribute := range vueScriptAttribute.FindAllStringSubmatch(attributes, -1) {
			if strings.EqualFold(attribute[1], "lang") {
				lang = strings.ToLower(attribute[2] + attribute[3] + attribute[4])
				break
			}
		}
		switch lang {
		case "ts", "typescript":
			return "TypeScript"
		case "", "js", "javascript":
			language = "JavaScript"
		default:
			return "" // Unsupported script language: do not guess.
		}
	}
	return language
}

func programmingSourceLanguage(root, path string) string {
	name := filepath.Base(path)
	if isUnderTopLevelOpsDir(root, path) {
		return ""
	}
	if shouldOmitFile(root, path, name) || isIdentityManifest(name) || isOperationalConfigFile(name) || isTextControlFile(name) || isConfigFileName(name) {
		return ""
	}
	if isUnderIgnoredDir(root, path, genericIgnoredDirs) || pathUsesCMakeBuildDir(relativePath(root, path), "") || sourceExtensionLanguage(path) == "C#" && isUnderCSharpObjDir(relativePath(root, path)) || isSharedWebResourcePath(relativePath(root, path)) {
		return ""
	}
	if filekind.Classify(path) != model.FileKindSource {
		return ""
	}
	return sourceFileLanguage(path)
}

func sourceWeights(files map[string]string) map[string]int64 {
	weights := make(map[string]int64)
	for _, language := range files {
		if language != "" {
			weights[language]++
		}
	}
	return weights
}
