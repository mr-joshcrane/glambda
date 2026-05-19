package glambda

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DriftStatus describes what kind of local changes exist for a package.
type DriftStatus struct {
	Package string
	Reason  string
}

// DriftResult is the outcome of a drift check. Callers can distinguish
// between "checked and found no drift" (Skipped=false, empty Drifts) and
// "could not check" (Skipped=true, SkipReason explains why).
type DriftResult struct {
	Drifts     []DriftStatus
	Skipped    bool
	SkipReason string
}

// CheckDrift inspects the handler file's imports against the current module
// and reports any local packages that have uncommitted or unpushed changes.
func CheckDrift(handlerPath string) DriftResult {
	modulePath, moduleRoot, err := findCurrentModule(handlerPath)
	if err != nil {
		return DriftResult{Skipped: true, SkipReason: err.Error()}
	}

	imports, err := parseLocalImports(handlerPath, modulePath)
	if err != nil {
		return DriftResult{Skipped: true, SkipReason: fmt.Sprintf("parsing imports: %s", err)}
	}
	if len(imports) == 0 {
		return DriftResult{Skipped: true, SkipReason: "no local module imports"}
	}

	if !isGitRepo(moduleRoot) {
		return DriftResult{Skipped: true, SkipReason: "not a git repository"}
	}

	var results []DriftStatus
	for _, imp := range imports {
		rel := strings.TrimPrefix(imp, modulePath+"/")
		dir := filepath.Join(moduleRoot, rel)
		if imp == modulePath {
			dir = moduleRoot
		}

		uncommitted := hasUncommittedChanges(moduleRoot, dir)
		unpushed := unpushedCommitCount(moduleRoot, dir)

		reason := driftReason(uncommitted, unpushed)
		if reason == "" {
			continue
		}
		results = append(results, DriftStatus{Package: imp, Reason: reason})
	}
	return DriftResult{Drifts: results}
}

func driftReason(uncommitted bool, unpushed int) string {
	switch {
	case uncommitted && unpushed > 0:
		return fmt.Sprintf("uncommitted changes + %d commits ahead", unpushed)
	case uncommitted:
		return "uncommitted changes"
	case unpushed > 0:
		return fmt.Sprintf("%d commits ahead of upstream", unpushed)
	default:
		return ""
	}
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
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive found in %s", gomodPath)
}

func parseLocalImports(handlerPath, modulePath string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, handlerPath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var imports []string
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == modulePath || strings.HasPrefix(path, modulePath+"/") {
			imports = append(imports, path)
		}
	}
	return imports, nil
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

// WarnDrift checks a handler for drift and returns the formatted warning.
func WarnDrift(handlerPath string) string {
	result := CheckDrift(handlerPath)
	return FormatDriftWarning(result.Drifts)
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
		fmt.Fprintf(&b, "    • %s  (%s)\n", d.Package, d.Reason)
	}

	b.WriteString("\n  Push your changes first, or use --dirty to deploy from local module state.\n")
	return b.String()
}
