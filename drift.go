package glambda

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DriftStatus describes what kind of local changes exist for a package.
type DriftStatus struct {
	Package     string
	Uncommitted bool
	Unpushed    int // number of commits ahead of upstream
}

// CheckDrift inspects the handler file's imports against the current module
// and reports any local packages that have uncommitted or unpushed changes.
// Returns nil if no drift is detected, or if we can't determine drift
// (not a git repo, no upstream, handler has no local imports).
func CheckDrift(handlerPath string) ([]DriftStatus, error) {
	modulePath, moduleRoot, err := findCurrentModule(handlerPath)
	if err != nil {
		return nil, nil // can't determine module — skip silently
	}

	imports, err := parseLocalImports(handlerPath, modulePath)
	if err != nil {
		return nil, nil // can't parse — skip silently
	}
	if len(imports) == 0 {
		return nil, nil // no local imports — nothing to check
	}

	if !isGitRepo(moduleRoot) {
		return nil, nil
	}

	var results []DriftStatus
	for _, imp := range imports {
		rel := strings.TrimPrefix(imp, modulePath+"/")
		dir := filepath.Join(moduleRoot, rel)

		status := DriftStatus{Package: imp}
		status.Uncommitted = hasUncommittedChanges(moduleRoot, dir)
		status.Unpushed = unpushedCommitCount(moduleRoot, dir)

		if status.Uncommitted || status.Unpushed > 0 {
			results = append(results, status)
		}
	}
	return results, nil
}

func findCurrentModule(handlerPath string) (modulePath string, moduleRoot string, err error) {
	abs, err := filepath.Abs(handlerPath)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(abs)

	for {
		gomod := filepath.Join(dir, "go.mod")
		if _, statErr := os.Stat(gomod); statErr == nil {
			modPath, parseErr := parseModulePath(gomod)
			if parseErr != nil {
				return "", "", parseErr
			}
			return modPath, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no go.mod found")
		}
		dir = parent
	}
}

func parseModulePath(gomodPath string) (string, error) {
	f, err := os.Open(gomodPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive found in %s", gomodPath)
}

func parseLocalImports(handlerPath, modulePath string) ([]string, error) {
	f, err := os.Open(handlerPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	inImportBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "import (" {
			inImportBlock = true
			continue
		}
		if inImportBlock && line == ")" {
			inImportBlock = false
			continue
		}

		var importPath string
		if inImportBlock {
			importPath = extractImportPath(line)
		} else if after, found := strings.CutPrefix(line, "import "); found {
			importPath = extractImportPath(after)
		}

		if importPath != "" && strings.HasPrefix(importPath, modulePath+"/") {
			imports = append(imports, importPath)
		}
	}
	return imports, scanner.Err()
}

func extractImportPath(line string) string {
	start := strings.IndexByte(line, '"')
	if start == -1 {
		return ""
	}
	end := strings.IndexByte(line[start+1:], '"')
	if end == -1 {
		return ""
	}
	return line[start+1 : start+1+end]
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

func hasUncommittedChanges(repoRoot, dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain", "--", dir)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func unpushedCommitCount(repoRoot, dir string) int {
	cmd := exec.Command("git", "log", "--oneline", "@{upstream}..HEAD", "--", dir)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// FormatDriftWarning produces the user-facing warning string for detected drift.
func FormatDriftWarning(drifts []DriftStatus) string {
	if len(drifts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n⚠ Local drift detected — deployed binary will use remote module state\n\n")
	b.WriteString("  Unpushed changes in imported packages:\n")

	for _, d := range drifts {
		switch {
		case d.Uncommitted && d.Unpushed > 0:
			fmt.Fprintf(&b, "    • %s  (uncommitted changes + %d commits ahead)\n", d.Package, d.Unpushed)
		case d.Uncommitted:
			fmt.Fprintf(&b, "    • %s  (uncommitted changes)\n", d.Package)
		default:
			fmt.Fprintf(&b, "    • %s  (%d commits ahead of upstream)\n", d.Package, d.Unpushed)
		}
	}

	b.WriteString("\n  Push your changes first, or use --dirty to deploy from local module state.\n")
	return b.String()
}
