package command_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-joshcrane/glambda/command"
)

func TestMain_ReturnsErrorAndHelpOnInvalidArgs(t *testing.T) {
	t.Parallel()
	tc := []struct {
		description string
		args        []string
	}{
		{
			description: "no args",
			args:        []string{},
		},

		{
			description: "invalid args",
			args:        []string{"invalid"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.description, func(t *testing.T) {
			buf := new(bytes.Buffer)
			err := command.Main(tt.args, command.WithOutput(buf))
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
			if !strings.Contains(buf.String(), "Usage:") {
				t.Errorf("Expected help message but got: %s", buf.String())
			}
		})
	}
}

func TestMain_SuccessfullyPackagesALambdaWithThePackageCommand(t *testing.T) {
	t.Parallel()
	handler := copyTestHandler(t)
	tempPath := t.TempDir() + "/package.zip"
	args := []string{"package", handler, "--output", tempPath}
	err := command.Main(args)
	if err != nil {
		t.Fatalf("Failed to package lambda: %v", err)
	}
	_, err = os.Stat(tempPath)
	if err != nil {
		t.Fatalf("Failed to find package.zip: %v", err)
	}
}

func copyTestHandler(t *testing.T) string {
	tempDir := t.TempDir()
	srcFile := "../testdata/correct_test_handler/main.go"
	dstFile := filepath.Join(tempDir, "main.go")

	src, err := os.Open(srcFile)
	if err != nil {
		t.Fatalf("failed to open source file: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstFile)
	if err != nil {
		t.Fatalf("failed to create destination file: %v", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	return dstFile
}
