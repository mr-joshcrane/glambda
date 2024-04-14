package glambda

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

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
	tempBootstrap := os.TempDir() + "/bootstrap"
	//go build -tags lambda.norpc -o bootstrap main.go
	cmd := exec.Command("go", "build", "-tags", "lambda.norpc", "-o", tempBootstrap, path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error building lambda function", string(out))
		return nil, err
	}

	data, err := os.ReadFile(tempBootstrap)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(tempBootstrap)
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
