package glambda

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// PackageTo takes a path to a file, attempts to build it for the ARM64 architecture
// and massages it into the format expected by AWS Lambda.
//
// The result is a zip file containing the executable binary within the context
// of a file system.
func PackageTo(path string, output io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "bootstrap")
	if err != nil {
		return err
	}
	defer os.Remove(tmpDir)
	sourceFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	tmpGoPath := tmpDir + "/main.go"
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
	err = cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("go", "mod", "tidy")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOMODCACHE="+tmpDir, "GOCACHE="+tmpDir)

	cmd.Dir = tmpDir
	err = cmd.Run()
	if err != nil {
		return err
	}
	executablePath := tmpDir + "/bootstrap"
	cmd = exec.Command("go", "build", "-tags", "lambda.norpc", "-o", executablePath, tmpGoPath)
	cmd.Dir = tmpDir
	cmd.Env = append(cmd.Env, "GOOS=linux", "GOARCH=arm64", "GOMODCACHE="+tmpDir, "GOCACHE="+tmpDir)
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
