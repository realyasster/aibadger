package scanner

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestScannerSourceBasedLanguageMatrix(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		languages []string
		primary   string
	}{
		{"Go", map[string]string{"go.mod": "module example.com/app\n", "cmd/main.go": "package main\n", "internal/server.go": "package server\n"}, []string{"Go"}, "Go"},
		{"Python", map[string]string{"pyproject.toml": "[project]\nname='app'\n", "src/app.py": "print('ok')\n"}, []string{"Python"}, "Python"},
		{"JavaScript", map[string]string{"package.json": `{"name":"app","devDependencies":{"typescript":"*"}}`, "tsconfig.json": "{}", "custom/worker.js": "export const worker = 1\n"}, []string{"JavaScript"}, "JavaScript"},
		{"TypeScript", map[string]string{"package.json": "{}", "tsconfig.json": "{}", "custom/worker.ts": "export const worker: number = 1\n", "pages/app.tsx": "export const App = () => <div/>\n"}, []string{"TypeScript"}, "TypeScript"},
		{"mixed Node", map[string]string{"package.json": `{"name":"mixed"}`, "custom/a.js": "export const a=1\n", "custom/b.jsx": "export const B=()=>null\n", "custom/c.ts": "export const c=1\n"}, []string{"JavaScript", "TypeScript"}, "JavaScript"},
		{"manifestless mixed", map[string]string{"a.rs": "fn main() {}\n", "b.rb": "puts 'ok'\n", "c.php": "<?php echo 'ok';\n"}, []string{"PHP", "Ruby", "Rust"}, "PHP"},
		{"Go manifest only", map[string]string{"go.mod": "module example.com/empty\n"}, []string{"Unknown"}, "Unknown"},
		{"Python manifest only", map[string]string{"pyproject.toml": "[project]\nname='empty'\n"}, []string{"Unknown"}, "Unknown"},
		{"Node manifest only", map[string]string{"package.json": `{"name":"empty","types":"index.d.ts","devDependencies":{"typescript":"*"}}`, "tsconfig.json": "{}"}, []string{"Unknown"}, "Unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range c.files {
				writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
			}
			topology, err := NewScanner(root).Scan()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(topology.Languages, c.languages) || topology.PrimaryLanguage != c.primary {
				t.Fatalf("languages=%v primary=%s modules=%+v", topology.Languages, topology.PrimaryLanguage, topology.Modules)
			}
		})
	}
}

func TestScannerGoTypeScriptPythonExample(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/mixed\n", "cmd/main.go": "package main\n", "internal/server.go": "package server\n",
		"web/package.json": `{"name":"frontend"}`, "web/tsconfig.json": "{}", "web/src/App.tsx": "export const App=()=>null\n", "web/src/main.ts": "export const main=1\n",
		"scripts/build.py": "print('build')\n",
	}
	for path, content := range files {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Go", "Python", "TypeScript"}) || topology.PrimaryLanguage != "Go" {
		t.Fatalf("languages=%v primary=%s", topology.Languages, topology.PrimaryLanguage)
	}
	if !reflect.DeepEqual(topology.Stack, []string{"Go Modules", "Node.js"}) || topology.Structure != "Multi-Module" {
		t.Fatalf("stack=%v structure=%s", topology.Stack, topology.Structure)
	}
	if frontend := findModule(topology.Modules, "frontend"); frontend == nil || frontend.Language != "TypeScript" {
		t.Fatalf("frontend=%+v", frontend)
	}
}

func TestLanguageWeightsDeduplicateModuleRecords(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "src", "a.go"), "package a\n")
	writeTestFile(t, filepath.Join(root, "src", "b.go"), "package a\n")
	module := model.Module{Language: "Go", SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source", Packages: []model.Package{{Path: "src"}, {Path: "src"}}}}}
	modules := []model.Module{module, module, {Language: "Go", LanguageSourceFiles: map[string]string{filepath.Join("src", "a.go"): "Go"}}}
	if got := sourceLanguageWeightsFromModules(modules, root)["Go"]; got != 2 {
		t.Fatalf("Go weight=%d want 2 unique files", got)
	}
}

func TestNodeWorkspaceOwnershipAndMixedWeights(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"package.json":             `{"name":"workspace","workspaces":["packages/*"]}`,
		"packages/ui/package.json": "{}", "packages/ui/widgets/app.ts": "export const app=1\n",
		"packages/api/package.json": `{"name":"api"}`, "packages/api/handlers/server.js": "export const server=1\n",
	}
	for path, content := range files {
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}
	modules, err := NewNodeDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 3 {
		t.Fatalf("modules=%+v", modules)
	}
	parent := findModule(modules, "workspace")
	if parent == nil || len(parent.LanguageSourceFiles) != 0 {
		t.Fatalf("parent counted child source: %+v", parent)
	}
	weights := sourceLanguageWeightsFromModules(append(modules, modules...), root)
	if weights["JavaScript"] != 1 || weights["TypeScript"] != 1 {
		t.Fatalf("weights=%v", weights)
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"JavaScript", "TypeScript"}) || topology.PrimaryLanguage != "JavaScript" {
		t.Fatalf("languages=%v primary=%s", topology.Languages, topology.PrimaryLanguage)
	}
	assertUniqueSurfacedPaths(t, topology)
}

func TestLanguageDetectionExcludesDependenciesOutputsAndContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{"name":"app"}`)
	writeTestFile(t, filepath.Join(root, "custom", "app.js"), "export const app=1\n")
	for _, dir := range []string{"node_modules", ".venv", "venv", "dist", "build", "target", "coverage", "out", ".next", "third-party", "cmake-build-debug"} {
		for i := 0; i < 3; i++ {
			writeTestFile(t, filepath.Join(root, dir, string(rune('a'+i))+".ts"), "export const a=1\n")
		}
		writeTestFile(t, filepath.Join(root, dir, "go.mod"), "module example.com/dependency\n")
	}
	writeTestFile(t, filepath.Join(root, "obj", "Generated.cs"), "class Generated {}\n")
	writeTestFile(t, filepath.Join(root, "obj", "Generated.dll"), "binary output")
	for _, path := range []string{"README.md", "package-lock.json", "tsconfig.json", "asset.png"} {
		writeTestFile(t, filepath.Join(root, path), "context\n")
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"JavaScript"}) || topology.PrimaryLanguage != "JavaScript" || !reflect.DeepEqual(topology.Stack, []string{"Node.js"}) {
		t.Fatalf("languages=%v primary=%s stack=%v", topology.Languages, topology.PrimaryLanguage, topology.Stack)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("dependency markers activated modules: %+v", topology.Modules)
	}
}

func TestVueSourceClassificationUsesScriptLanguage(t *testing.T) {
	for _, c := range []struct{ source, want string }{
		{`<script setup lang="ts">const x: number = 1</script>`, "TypeScript"},
		{`<script>export default {}</script>`, "JavaScript"},
		{`<template><h1>Hello</h1></template>`, "Vue"},
		{`<!-- <script lang="ts"> --> <script lang="js">export default {}</script>`, "JavaScript"},
		{`<script lang="coffee">x = 1</script>`, ""},
		{`<script data-lang="ts">export default {}</script>`, "JavaScript"},
		{`<script src="foo lang=ts">export default {}</script>`, "JavaScript"},
		{`<script>const markup = '<script lang="ts">';</script>`, "JavaScript"},
		{`<script>export default {}</script><script setup lang="ts">const x: number = 1</script>`, "TypeScript"},
	} {
		root := t.TempDir()
		file := filepath.Join(root, "components", "App.vue")
		writeTestFile(t, file, c.source)
		if got := sourceFileLanguage(file); got != c.want {
			t.Fatalf("%q language=%q want=%q", c.source, got, c.want)
		}
		writeTestFile(t, filepath.Join(root, "package.json"), `{"name":"vue-app"}`)
		modules, err := NewNodeDetector().Detect(root)
		if err != nil || len(modules) != 1 || modules[0].FileCount != 1 {
			t.Fatalf("Vue component not counted: modules=%+v err=%v", modules, err)
		}
		wantModule := c.want
		if wantModule == "" {
			wantModule = "Unknown"
		}
		if modules[0].Language != wantModule {
			t.Fatalf("Vue module language=%s want=%s", modules[0].Language, wantModule)
		}
	}
}

func TestNodeAllJSAndTSExtensionsVoteOnce(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{"name":"mixed-extensions","types":"missing.d.ts","devDependencies":{"typescript":"*"}}`)
	for _, ext := range []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"} {
		writeTestFile(t, filepath.Join(root, "custom", "entry"+ext), "export const entry=1\n")
	}
	modules, err := NewNodeDetector().Detect(root)
	if err != nil || len(modules) != 1 {
		t.Fatalf("modules=%+v err=%v", modules, err)
	}
	weights := sourceLanguageWeightsFromModules(modules, root)
	if weights["JavaScript"] != 4 || weights["TypeScript"] != 4 || modules[0].Language != "JavaScript" {
		t.Fatalf("source weights=%v module=%s", weights, modules[0].Language)
	}
}

func TestLegacyNodePackagesAlsoCountBothSourceLanguages(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "custom", "app.js"), "export const app=1\n")
	writeTestFile(t, filepath.Join(root, "custom", "app.ts"), "export const app=1\n")
	module := model.Module{Language: "JavaScript", SourceRoots: []model.SourceRoot{{Role: "Main Source", Packages: []model.Package{{Path: "custom"}}}}}
	weights := sourceLanguageWeightsFromModules([]model.Module{module, module}, root)
	if weights["JavaScript"] != 1 || weights["TypeScript"] != 1 {
		t.Fatalf("weights=%v", weights)
	}
}

func TestAmbiguousExtensionsDoNotGuessLanguage(t *testing.T) {
	for _, path := range []string{"ambiguous.h", "ambiguous.vh", "ambiguous.v", "ambiguous.f", "README.md", "package.json"} {
		if language := sourceExtensionLanguage(path); language != "" {
			t.Fatalf("%s incorrectly guessed as %s", path, language)
		}
	}
}

func TestSourceRecordsAreNotSerialized(t *testing.T) {
	data, err := json.Marshal(model.Module{LanguageSourceFiles: map[string]string{"secret/internal.go": "Go"}})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["LanguageSourceFiles"]; ok {
		t.Fatalf("internal source records serialized: %s", data)
	}
}
