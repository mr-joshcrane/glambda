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

func TestParseEnvironment(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description string
		input       string
		want        map[string]string
		wantErr     bool
	}{
		{
			description: "empty string",
			input:       "",
			want:        map[string]string{},
			wantErr:     false,
		},
		{
			description: "single pair",
			input:       "KEY1=value1",
			want:        map[string]string{"KEY1": "value1"},
			wantErr:     false,
		},
		{
			description: "multiple pairs",
			input:       "KEY1=value1,KEY2=value2,KEY3=value3",
			want:        map[string]string{"KEY1": "value1", "KEY2": "value2", "KEY3": "value3"},
			wantErr:     false,
		},
		{
			description: "with spaces",
			input:       " KEY1 = value1 , KEY2 = value2 ",
			want:        map[string]string{"KEY1": "value1", "KEY2": "value2"},
			wantErr:     false,
		},
		{
			description: "value with equals sign",
			input:       "KEY1=value=with=equals",
			want:        map[string]string{"KEY1": "value=with=equals"},
			wantErr:     false,
		},
		{
			description: "invalid format missing value",
			input:       "KEY1",
			wantErr:     true,
		},
		{
			description: "invalid format empty key",
			input:       "=value",
			wantErr:     true,
		},
		{
			description: "invalid format one valid one invalid",
			input:       "KEY1=value1,INVALID",
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			got, err := command.ParseEnvironment(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Errorf("expected %d items, got %d", len(tc.want), len(got))
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("expected %s=%s, got %s=%s", k, v, k, got[k])
				}
			}
		})
	}
}
