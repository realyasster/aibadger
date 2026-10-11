package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/util"
)

// Scan all safe source directories, retaining conventional root roles. A nested
// package owns its subtree; workspace parent modules must not count it again.
func (n *NodeDetector) scanModuleSources(module *model.Module, projectRoot, moduleRoot string) {
	detector := NewGenericDetector()
	detector.Exclusions = n.Exclusions
	// Node's existing cap counts source files, not subdirectory entries.
	perDirectory := make(map[string]int)
	detector.skipDirectory = func(path string) bool {
		return isUnderTopLevelOpsDir(projectRoot, path) || util.FileExists(filepath.Join(path, "package.json"))
	}
	module.LanguageSourceFiles = make(map[string]string)
	roots := make(map[string]*model.SourceRoot)
	packages := make(map[string]map[string]*model.Package)
	_ = detector.walkFiles(moduleRoot, func(path string, entry os.DirEntry, info os.FileInfo) {
		if !isNodeSourceFile(entry.Name()) || shouldOmitFile(projectRoot, path, entry.Name()) {
			return
		}
		perDirectory[filepath.Dir(path)]++
		if n.maxFilesPerDir > 0 && perDirectory[filepath.Dir(path)] > n.maxFilesPerDir {
			return
		}
		if language := programmingSourceLanguage(projectRoot, path); language != "" {
			module.LanguageSourceFiles[normalizeTopologyFilePath(relativePath(projectRoot, path))] = language
		}
		rel := relativePath(moduleRoot, path)
		rootName := ""
		if dir := filepath.Dir(rel); dir != "." {
			rootName = strings.Split(dir, string(filepath.Separator))[0]
		}
		root := roots[rootName]
		if root == nil {
			role := n.inferSourceRole(rootName)
			if rootName != "" && rootName != "src" && rootName != "app" && rootName != "lib" && rootName != "server" && rootName != "test" && rootName != "tests" {
				role = "Additional Source"
			}
			root = &model.SourceRoot{Path: joinRelativePath(module.Path, rootName), Role: role}
			roots[rootName] = root
			packages[rootName] = make(map[string]*model.Package)
		}
		root.FileCount++
		recordNodeFile(packages[rootName], filepath.Join(moduleRoot, rootName), projectRoot, path, entry.Name(), info.Size())
	})
	names := make([]string, 0, len(roots))
	for name := range roots {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		root := roots[name]
		for _, pkg := range packages[name] {
			root.Packages = append(root.Packages, *pkg)
			for _, file := range pkg.TopFiles {
				module.TopFiles = addTopFile(module.TopFiles, file, moduleTopFileLimit(module.Path, maxPackageTopFiles))
			}
		}
		sort.Slice(root.Packages, func(i, j int) bool { return packageSortKey(root.Packages[i]) < packageSortKey(root.Packages[j]) })
		module.SourceRoots = append(module.SourceRoots, *root)
		module.FileCount += root.FileCount
	}
}

func crossesNestedNodeProject(projectRoot, moduleRoot, rel string) bool {
	dir := normalizeRelativeDir(filepath.Dir(rel))
	local, ok := relativeWithin(moduleRoot, dir)
	if !ok || local == "" {
		return false
	}
	current := moduleRoot
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		current = joinRelativePath(current, part)
		if util.FileExists(filepath.Join(projectRoot, current, "package.json")) {
			return true
		}
	}
	return false
}
