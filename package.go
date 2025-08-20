package glambda

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// PackageTo takes a path to a file, attempts to build it for the ARM64 architecture
// and massages it into the format expected by AWS Lambda.
//
// The result is a zip file containing the executable binary within the context
// of a file system.
func PackageTo(path string, output io.Writer) error {
	absolutepath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	dir := filepath.Dir(absolutepath)
	sourceFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	cmd := exec.Command("go", "mod", "init", "main")
	cmd.Dir = dir
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	envs := os.Environ()
	GOMODCACHE := os.Getenv("GOMODCACHE")
	if GOMODCACHE == "" {
		GOMODCACHE = filepath.Join(os.Getenv("HOME"), "go/pkg/mod")
	}
	GOCACHE := os.Getenv("GOCACHE")
	if GOCACHE == "" {
		GOCACHE = filepath.Join(os.Getenv("HOME"), ".cache/go-build")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error tidying go module: %s", string(out))
	}
	tempdir := os.TempDir()
	executablePath := tempdir + "/bootstrap"
	cmd = exec.Command("go", "build", "-tags", "lambda.norpc", "-o", executablePath, absolutepath)
	cmd.Dir = dir
	cmd.Env = append(envs, "GOOS=linux", "GOARCH=arm64", "GOMODCACHE="+GOMODCACHE, "GOCACHE="+GOCACHE)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error building lambda function: %w, %s", err, msg)
	}
	zipWriter := zip.NewWriter(output)
	header := &zip.FileHeader{
		Name:   "bootstrap",
		Method: zip.Deflate,
	}
	header.SetMode(0755)

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
	// Close the ZIP writer to finalize the archive
	return zipWriter.Close()
}
