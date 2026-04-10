package command

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-joshcrane/glambda"
	"github.com/spf13/cobra"
)

type CommandOptions func(*cobra.Command) error

func WithOutput(w io.Writer) CommandOptions {
	return func(cmd *cobra.Command) error {
		cmd.SetOutput(w)
		return nil
	}
}

func WithPackagePath(path string) CommandOptions {
	return func(cmd *cobra.Command) error {
		return cmd.Flags().Set("output", path)
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
		PackageCommand(),
		ListCommand(),
		PlanCommand(),
		ApplyCommand(),
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
			timeout, _ := cmd.Flags().GetInt("timeout")
			memorySize, _ := cmd.Flags().GetInt("memory-size")
			description, _ := cmd.Flags().GetString("description")
			environment, _ := cmd.Flags().GetString("environment")

			opts := []glambda.DeployOptions{
				glambda.WithManagedPolicies(managedPolicies),
				glambda.WithInlinePolicy(inlinePolicy),
				glambda.WithResourcePolicy(resourcePolicy),
			}

			if timeout > 0 {
				opts = append(opts, glambda.WithTimeout(timeout))
			}
			if memorySize > 0 {
				opts = append(opts, glambda.WithMemorySize(memorySize))
			}
			if description != "" {
				opts = append(opts, glambda.WithDescription(description))
			}
			if environment != "" {
				if environment == "none" {
					opts = append(opts, glambda.WithEnvironment(map[string]string{}))
				} else {
					env, err := ParseEnvironment(environment)
					if err != nil {
						return err
					}
					opts = append(opts, glambda.WithEnvironment(env))
				}
			}

			return glambda.Deploy(functionName, sourceCodePath, opts...)
		},
	}
	deployCmd.Flags().String("managed-policies", "", "Managed policies to attach to the lambda function.")
	deployCmd.Flags().String("inline-policy", "", "Inline policy to attach to the lambda function.")
	deployCmd.Flags().String("resource-policy", "", "Resource policy to attach to the lambda function.")
	deployCmd.Flags().Int("timeout", 0, "Function timeout in seconds (1-900)")
	deployCmd.Flags().Int("memory-size", 0, "Function memory in MB (128-10240)")
	deployCmd.Flags().String("description", "", "Function description")
	deployCmd.Flags().String("environment", "", "Environment variables as KEY1=VAL1,KEY2=VAL2 or 'none' to clear all")
	return deployCmd
}

// ParseEnvironment converts a comma-separated string of KEY=VALUE pairs into a map.
// Format: "KEY1=VAL1,KEY2=VAL2"
func ParseEnvironment(envString string) (map[string]string, error) {
	env := make(map[string]string)
	if envString == "" {
		return env, nil
	}

	pairs := strings.Split(envString, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("empty environment variable key in: %s", pair)
		}
		env[key] = value
	}
	return env, nil
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

func PackageCommand() *cobra.Command {
	var packageCmd = &cobra.Command{
		Use:          "package sourceCodePath",
		Short:        "Package a Go binary as a ZIP'd bundle ready to upload to AWS.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Example:      `glambda package /path/to/sourceCode.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceCodePath := args[0]
			sourceCodePath, err := filepath.Abs(sourceCodePath)
			if err != nil {
				return fmt.Errorf("error getting path for source code, %w", err)
			}
			outputPath, err := cmd.Flags().GetString("output")
			if err != nil {
				return fmt.Errorf("error getting output path, %w", err)
			}
			outputFile, err := os.Create(outputPath)
			if err != nil {
				return err
			}
			defer outputFile.Close()
			err = glambda.PackageTo(sourceCodePath, outputFile)
			if err != nil {
				return fmt.Errorf("error packaging lambda function, %w", err)
			}
			fmt.Println("File successfully written to", outputPath)
			return nil
		},
	}
	packageCmd.Flags().String("output", "bootstrap", "Path to write the packaged lambda function.")
	return packageCmd
}

func ListCommand() *cobra.Command {
	var listCmd = &cobra.Command{
		Use:          "list",
		Short:        "List all Lambda functions deployed by glambda.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Example:      `glambda list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			functions, err := glambda.List()
			if err != nil {
				return err
			}
			if len(functions) == 0 {
				fmt.Println("No glambda-managed Lambda functions found.")
				return nil
			}
			fmt.Printf("%-30s %-20s %-30s\n", "NAME", "RUNTIME", "LAST MODIFIED")
			fmt.Println(strings.Repeat("-", 80))
			for _, fn := range functions {
				fmt.Printf("%-30s %-20s %-30s\n", fn.Name, fn.Runtime, fn.LastModified)
			}
			return nil
		},
	}
	return listCmd
}

func PlanCommand() *cobra.Command {
	var planCmd = &cobra.Command{
		Use:          "plan",
		Short:        "Show what changes would be made to reconcile AWS with glambda.toml.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Example:      `glambda plan --config glambda.toml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			cfg, err := glambda.LoadConfig(configPath)
			if err != nil {
				return err
			}
			plan, err := glambda.PlanFromConfig(cfg)
			if err != nil {
				return err
			}
			printPlan(plan)
			return nil
		},
	}
	planCmd.Flags().String("config", "glambda.toml", "Path to the glambda.toml config file")
	return planCmd
}

func ApplyCommand() *cobra.Command {
	var applyCmd = &cobra.Command{
		Use:          "apply",
		Short:        "Apply changes to reconcile AWS with glambda.toml.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Example:      `glambda apply --config glambda.toml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			autoApprove, _ := cmd.Flags().GetBool("auto-approve")
			cfg, err := glambda.LoadConfig(configPath)
			if err != nil {
				return err
			}
			plan, err := glambda.PlanFromConfig(cfg)
			if err != nil {
				return err
			}
			printPlan(plan)
			if !plan.HasChanges() {
				return nil
			}
			if !autoApprove {
				fmt.Print("\nDo you want to apply these changes? (yes/no): ")
				var response string
				fmt.Scanln(&response)
				if response != "yes" {
					fmt.Println("Apply cancelled.")
					return nil
				}
			}
			err = glambda.ExecutePlan(plan, cfg)
			if err != nil {
				return err
			}
			fmt.Println("\nApply complete.")
			return nil
		},
	}
	applyCmd.Flags().String("config", "glambda.toml", "Path to the glambda.toml config file")
	applyCmd.Flags().Bool("auto-approve", false, "Skip interactive approval before applying")
	return applyCmd
}

func printPlan(plan glambda.Plan) {
	creates, updates, deletes := plan.Summary()
	fmt.Printf("Project: %s\n\n", plan.ProjectName)
	if !plan.HasChanges() {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return
	}
	for _, item := range plan.Items {
		switch item.Action {
		case glambda.PlanCreate:
			fmt.Printf("  + CREATE  %-30s (%s)\n", item.Name, item.Reason)
		case glambda.PlanUpdate:
			fmt.Printf("  ~ UPDATE  %-30s (%s)\n", item.Name, item.Reason)
		case glambda.PlanDelete:
			fmt.Printf("  - DELETE  %-30s (%s)\n", item.Name, item.Reason)
		}
	}
	fmt.Printf("\nPlan: %d to create, %d to update, %d to delete.\n", creates, updates, deletes)
}
