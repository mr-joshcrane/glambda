package glambda_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-joshcrane/glambda"
)

func TestCheckDrift_NoDriftWhenNoLocalImports(t *testing.T) {
	t.Parallel()
	handler := filepath.Join(t.TempDir(), "main.go")
	err := os.WriteFile(handler, []byte(`package main

import "fmt"

func main() { fmt.Println("hi") }
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	drifts := glambda.CheckDrift(handler)
	if len(drifts) != 0 {
		t.Errorf("expected no drifts for handler with no local imports, got %d", len(drifts))
	}
}

func TestCheckDrift_NoDriftWhenNotInGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	pkgDir := filepath.Join(dir, "pkg")
	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(pkgDir, "lib.go"), []byte("package pkg\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	handler := filepath.Join(dir, "main.go")
	err = os.WriteFile(handler, []byte(`package main

import "example.com/test/pkg"

func main() { _ = pkg.X }
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	drifts := glambda.CheckDrift(handler)
	if len(drifts) != 0 {
		t.Errorf("expected no drifts outside git repo, got %d", len(drifts))
	}
}

func TestCheckDrift_DetectsUncommittedChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")

	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	pkgDir := filepath.Join(dir, "pkg")
	err = os.MkdirAll(pkgDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(pkgDir, "lib.go"), []byte("package pkg\n\nvar X = 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	handler := filepath.Join(dir, "main.go")
	err = os.WriteFile(handler, []byte(`package main

import "example.com/test/pkg"

func main() { _ = pkg.X }
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")

	err = os.WriteFile(filepath.Join(pkgDir, "lib.go"), []byte("package pkg\n\nvar X = 2\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	drifts := glambda.CheckDrift(handler)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if !strings.Contains(drifts[0].Reason, "uncommitted") {
		t.Errorf("expected reason to mention uncommitted, got %s", drifts[0].Reason)
	}
	if !strings.Contains(drifts[0].Package, "example.com/test/pkg") {
		t.Errorf("expected package to contain example.com/test/pkg, got %s", drifts[0].Package)
	}
}

func TestFormatDriftWarning_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()
	result := glambda.FormatDriftWarning(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatDriftWarning_FormatsCorrectly(t *testing.T) {
	t.Parallel()
	drifts := []glambda.DriftStatus{
		{Package: "example.com/test/pkg", Reason: "uncommitted changes"},
		{Package: "example.com/test/auth", Reason: "3 commits ahead of upstream"},
	}
	result := glambda.FormatDriftWarning(drifts)
	if !strings.Contains(result, "Local drift detected") {
		t.Error("expected warning header")
	}
	if !strings.Contains(result, "example.com/test/pkg") {
		t.Error("expected pkg in output")
	}
	if !strings.Contains(result, "uncommitted changes") {
		t.Error("expected uncommitted label")
	}
	if !strings.Contains(result, "3 commits ahead") {
		t.Error("expected commits ahead label")
	}
	if !strings.Contains(result, "--dirty") {
		t.Error("expected --dirty suggestion")
	}
}

func TestCheckDrift_DetectsRootModuleImport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")

	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package test\n\nvar X = 1\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(dir, "cmd")
	err = os.MkdirAll(subDir, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	handler := filepath.Join(subDir, "main.go")
	err = os.WriteFile(handler, []byte(`package main

import "example.com/test"

func main() { _ = test.X }
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial")

	err = os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package test\n\nvar X = 2\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	drifts := glambda.CheckDrift(handler)
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift for root module import, got %d", len(drifts))
	}
	if drifts[0].Package != "example.com/test" {
		t.Errorf("expected package example.com/test, got %s", drifts[0].Package)
	}
}

func TestParseLocalImports_ExtractsModuleImports(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/repo\n\ngo 1.22\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	handler := filepath.Join(dir, "main.go")
	err = os.WriteFile(handler, []byte(`package main

import (
	"fmt"
	"github.com/test/repo/internal/auth"
	"github.com/test/repo/pkg/config"
	"github.com/other/lib"
)

func main() {
	fmt.Println(auth.X, config.Y)
}
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	drifts := glambda.CheckDrift(handler)
	if len(drifts) != 0 {
		t.Errorf("expected no drifts (no git), got %d", len(drifts))
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
