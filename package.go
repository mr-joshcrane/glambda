package glambda

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// Package takes a path to a file, attempts to build it for the ARM64 architecture
// and massages it into the format expected by AWS Lambda.
//
// The result is a zip file containing the executable binary within the context
// of a file system.
func Package(path string) ([]byte, error) {
	data, err := buildBinary(path)
	if err != nil {
		return nil, err
	}
	return zipCode(data)
}

func buildBinary(path string) ([]byte, error) {
	err := os.Setenv("GOOS", "linux")
	if err != nil {
		return nil, err
	}
	err = os.Setenv("GOARCH", "arm64")
	if err != nil {
		return nil, err
	}
	tempBootstrap, err := os.MkdirTemp("", "bootstrap")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempBootstrap)

	tempBootstrap = tempBootstrap + "/bootstrap"

	//go build -tags lambda.norpc -o bootstrap main.go
	cmd := exec.Command("go", "build", "-tags", "lambda.norpc", "-o", tempBootstrap, path)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error building lambda function: %w, %s", err, msg)
	}

	data, err := os.ReadFile(tempBootstrap)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func zipCode(code []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	header := &zip.FileHeader{
		Name:   "bootstrap",
		Method: zip.Deflate,
	}
	header.SetMode(0755)

	// Add a file to the ZIP archive
	f, err := zipWriter.CreateHeader(header)
	if err != nil {
		return nil, fmt.Errorf("failed to create zip file header: %v", err)
	}
	_, err = f.Write(code)
	if err != nil {
		return nil, fmt.Errorf("failed to write code to zip file: %v", err)
	}
	// Close the ZIP writer to finalize the archive
	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %v", err)
	}
	return buf.Bytes(), nil
}
