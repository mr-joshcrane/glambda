package glambda_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mr-joshcrane/glambda"
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
			description: "partial command",
			args:        []string{"deploy", "myFunctionName"},
		},
		{
			description: "invalid args",
			args:        []string{"invalid"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.description, func(t *testing.T) {
			buf := new(bytes.Buffer)
			err := glambda.Main(tt.args, glambda.WithOutput(buf))
			if err == nil {
				t.Errorf("Expected error but got nil")
			}
			if !strings.Contains(buf.String(), "Usage:") {
				t.Errorf("Expected help message but got: %s", buf.String())
			}
		})
	}
}
