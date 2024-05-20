package command_test

import (
	"bytes"
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
