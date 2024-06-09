package command_test

import (
	"bytes"
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
	tempPath := t.TempDir() + "/package.zip"
	err := os.Chdir("../testdata/correct_test_handler")
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	absPath, err := filepath.Abs("main.go")
	if err != nil {
		t.Fatalf("Test setup for correct_test_handler failed: %v", err)
	}
	args := []string{"package", absPath, "--output", tempPath}
	err = command.Main(args)
	if err != nil {
		t.Fatalf("Failed to package lambda: %v", err)
	}
	_, err = os.Stat(tempPath)
	if err != nil {
		t.Fatalf("Failed to find package.zip: %v", err)
	}
}
