package glambda

import (
	"github.com/spf13/cobra"
)

func Main(args []string) error {
	var rootCmd = &cobra.Command{Use: "glambda"}
	commands := []func([]string) *cobra.Command{
		DeployCommand,
	}

	for _, cmd := range commands {
		rootCmd.AddCommand(cmd(args))
	}
	return rootCmd.Execute()
}

func DeployCommand(args []string) *cobra.Command {
	var deployCmd = &cobra.Command{
		Use:          "deploy [name] [source]",
		Short:        "Package a Go binary and upload it as a lambda function.",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			functionName := args[0]
			sourceCodePath := args[1]
			managedPolicies, _ := cmd.Flags().GetString("managed-policies")
			inlinePolicy, _ := cmd.Flags().GetString("inline-policy")
			resourcePolicy, _ := cmd.Flags().GetString("resource-policy")
			return Deploy(functionName, sourceCodePath,
				WithManagedPolicies(managedPolicies),
				WithInlinePolicy(inlinePolicy),
				WithResourcePolicy(resourcePolicy),
			)
		},
	}
	deployCmd.Flags().String("managed-policies", "", "Managed policies to attach to the lambda function.")
	deployCmd.Flags().String("inline-policy", "", "Inline policy to attach to the lambda function.")
	deployCmd.Flags().String("resource-policy", "", "Resource policy to attach to the lambda function.")
	return deployCmd
}
