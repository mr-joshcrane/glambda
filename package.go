package glambda

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// PackageLocalTo builds the handler from within its parent module, preserving
// all local dependencies. Use this when you want the deployed binary to reflect
// unpushed local module state (--dirty mode).
func PackageLocalTo(path string, output io.Writer) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	_, moduleRoot, err := findCurrentModule(absPath)
	if err != nil {
		return fmt.Errorf("--dirty requires handler to be within a Go module: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "glambda-dirty")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	executablePath := filepath.Join(tmpDir, "bootstrap")
	cmd := exec.Command("go", "build", "-tags", "lambda.norpc", "-o", executablePath, absPath)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error building lambda function: %w, %s", err, string(out))
	}

	return zipBootstrap(executablePath, output)
}

// PackageTo takes a path to a file, attempts to build it for the ARM64 architecture
// and massages it into the format expected by AWS Lambda (--pure mode).
//
// The handler is built in isolation — dependencies are resolved from their
// published remote state, not local disk.
func PackageTo(path string, output io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "bootstrap")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	sourceFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	tmpGoPath := filepath.Join(tmpDir, "main.go")
	tmpGoFile, err := os.Create(tmpGoPath)
	if err != nil {
		return err
	}
	defer tmpGoFile.Close()

	_, err = io.Copy(tmpGoFile, sourceFile)
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "mod", "init", "main")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error initializing go module: %w, %s", err, string(out))
	}

	envs := os.Environ()
	GOMODCACHE := os.Getenv("GOMODCACHE")
	if GOMODCACHE == "" {
		GOMODCACHE = filepath.Join(os.Getenv("HOME"), "go/pkg/mod")
	}
	GOCACHE := os.Getenv("GOCACHE")
	if GOCACHE == "" {
		GOCACHE = filepath.Join(os.Getenv("HOME"), ".cache/go-build")
	}

	cmd = exec.Command("go", "mod", "tidy")
	envs = append(envs, "GOMODCACHE="+GOMODCACHE, "GOCACHE="+GOCACHE, "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	cmd.Env = envs
	cmd.Dir = tmpDir
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error tidying go module: %w, %s", err, string(out))
	}

	executablePath := filepath.Join(tmpDir, "bootstrap")
	cmd = exec.Command("go", "build", "-tags", "lambda.norpc", "-o", executablePath, tmpGoPath)
	cmd.Dir = tmpDir
	cmd.Env = envs
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error building lambda function: %w, %s", err, string(out))
	}

	return zipBootstrap(executablePath, output)
}

func zipBootstrap(executablePath string, output io.Writer) error {
	zipWriter := zip.NewWriter(output)
	header := &zip.FileHeader{
		Name:   "bootstrap",
		Method: zip.Deflate,
	}
	header.SetMode(0o755)

	zipContents, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create zip file header: %w", err)
	}
	executable, err := os.Open(executablePath)
	if err != nil {
		return fmt.Errorf("failed to open executable: %w", err)
	}
	defer executable.Close()

	_, err = io.Copy(zipContents, executable)
	if err != nil {
		return fmt.Errorf("failed to write code to zip file: %w", err)
	}
	return zipWriter.Close()
}
