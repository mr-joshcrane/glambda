package command

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/mr-joshcrane/glambda"
)

type CommandOptions func(*cobra.Command) error

func WithOutput(w io.Writer) CommandOptions {
	return func(cmd *cobra.Command) error {
		cmd.SetOutput(w)
		return nil
	}
}

func Main(args []string, opts ...CommandOptions) error {
	var rootCmd = &cobra.Command{
		Use:   "glambda",
		Short: "A tool for deploying Go binaries as AWS Lambda functions.",
	}
	rootCmd.SetArgs(args)
	commands := []*cobra.Command{
		DeployCommand(),
		DeleteCommand(),
	}
	for _, opt := range opts {
		err := opt(rootCmd)
		if err != nil {
			return err
		}
	}
	rootCmd.AddCommand(commands...)
	if len(args) == 0 {
		rootCmd.Printf(rootCmd.UsageString())
		return fmt.Errorf("no command provided")
	}
	rootCmd.SetArgs(args)
	_, _, err := rootCmd.Find(args)
	if err != nil {
		rootCmd.Printf(rootCmd.UsageString())
		return err
	}
	rootCmd.SetHelpCommand(&cobra.Command{Use: "no-help", Run: func(cmd *cobra.Command, args []string) {}})
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	return rootCmd.Execute()
}

func DeployCommand() *cobra.Command {
	var deployCmd = &cobra.Command{
		Use:          "deploy functionName sourceCodePath",
		Short:        "Package a Go binary and upload it as a lambda function.",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		Example:      `glambda deploy myFunctionName /path/to/sourceCode.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			functionName := args[0]
			sourceCodePath := args[1]
			managedPolicies, _ := cmd.Flags().GetString("managed-policies")
			inlinePolicy, _ := cmd.Flags().GetString("inline-policy")
			resourcePolicy, _ := cmd.Flags().GetString("resource-policy")
			return glambda.Deploy(functionName, sourceCodePath,
				glambda.WithManagedPolicies(managedPolicies),
				glambda.WithInlinePolicy(inlinePolicy),
				glambda.WithResourcePolicy(resourcePolicy),
			)
		},
	}
	deployCmd.Flags().String("managed-policies", "", "Managed policies to attach to the lambda function.")
	deployCmd.Flags().String("inline-policy", "", "Inline policy to attach to the lambda function.")
	deployCmd.Flags().String("resource-policy", "", "Resource policy to attach to the lambda function.")
	return deployCmd
}

func DeleteCommand() *cobra.Command {
	var deleteCmd = &cobra.Command{
		Use:          "delete functionName",
		Short:        "Delete a lambda function.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Example:      `glambda delete myFunctionName`,
		RunE: func(cmd *cobra.Command, args []string) error {
			functionName := args[0]
			return glambda.Delete(functionName)
		},
	}
	return deleteCmd
}
