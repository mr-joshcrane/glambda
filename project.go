package glambda

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ProjectConfig represents the top-level structure of a glambda.toml file.
// It defines a project name that groups lambdas together, and the set of
// lambda functions that should exist in AWS for this project.
type ProjectConfig struct {
	Project ProjectMeta        `toml:"project"`
	Lambda  []LambdaDefinition `toml:"lambda"`
}

// ProjectMeta holds metadata about the project, primarily the name used
// to group and identify lambdas belonging to this project in AWS tags.
type ProjectMeta struct {
	Name string `toml:"name"`
}

// LambdaDefinition represents a single lambda function as defined in a
// glambda.toml configuration file. It captures all the information needed
// to deploy and configure a lambda function.
type LambdaDefinition struct {
	Name            string            `toml:"name"`
	Handler         string            `toml:"handler"`
	Timeout         int               `toml:"timeout"`
	MemorySize      int               `toml:"memory-size"`
	Description     string            `toml:"description"`
	ManagedPolicies []string          `toml:"managed-policies"`
	InlinePolicy    string            `toml:"inline-policy"`
	ResourcePolicy  string            `toml:"resource-policy"`
	Environment     map[string]string `toml:"environment"`
}

// LoadConfig reads and parses a glambda.toml file from the given path.
// It validates that the config has a project name and at least one lambda
// definition, and that each lambda has a name and handler path.
func LoadConfig(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("reading config file: %w", err)
	}
	return ParseConfig(string(data))
}

// ParseConfig parses TOML content into a ProjectConfig, performing
// validation on the result.
func ParseConfig(content string) (ProjectConfig, error) {
	var config ProjectConfig
	_, err := toml.Decode(content, &config)
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("parsing config: %w", err)
	}
	err = validateConfig(config)
	if err != nil {
		return ProjectConfig{}, err
	}
	return config, nil
}

func validateConfig(config ProjectConfig) error {
	if config.Project.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if len(config.Lambda) == 0 {
		return fmt.Errorf("at least one [[lambda]] definition is required")
	}
	seen := make(map[string]bool)
	for i, l := range config.Lambda {
		if l.Name == "" {
			return fmt.Errorf("lambda[%d]: name is required", i)
		}
		if l.Handler == "" {
			return fmt.Errorf("lambda[%d] (%s): handler is required", i, l.Name)
		}
		if seen[l.Name] {
			return fmt.Errorf("lambda[%d]: duplicate name %q", i, l.Name)
		}
		seen[l.Name] = true
	}
	return nil
}

// ConfigHash computes a deterministic hash of a LambdaDefinition's
// configuration. This hash is stored as an AWS tag on the lambda function
// and used to detect when the local config has changed relative to what
// was last deployed.
func ConfigHash(def LambdaDefinition) string {
	h := sha256.New()
	fmt.Fprintf(h, "name=%s\n", def.Name)
	fmt.Fprintf(h, "handler=%s\n", def.Handler)
	fmt.Fprintf(h, "timeout=%d\n", def.Timeout)
	fmt.Fprintf(h, "memory-size=%d\n", def.MemorySize)
	fmt.Fprintf(h, "description=%s\n", def.Description)
	fmt.Fprintf(h, "inline-policy=%s\n", def.InlinePolicy)
	fmt.Fprintf(h, "resource-policy=%s\n", def.ResourcePolicy)

	// Sort managed policies for determinism
	policies := make([]string, len(def.ManagedPolicies))
	copy(policies, def.ManagedPolicies)
	sort.Strings(policies)
	fmt.Fprintf(h, "managed-policies=%s\n", strings.Join(policies, ","))

	// Sort environment keys for determinism
	if len(def.Environment) > 0 {
		keys := make([]string, 0, len(def.Environment))
		for k := range def.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "env.%s=%s\n", k, def.Environment[k])
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// ToDeployOptions converts a LambdaDefinition into the slice of DeployOptions
// that the existing Deploy function expects. This bridges the declarative config
// with the existing imperative deployment machinery.
func (def LambdaDefinition) ToDeployOptions() []DeployOptions {
	var opts []DeployOptions
	if len(def.ManagedPolicies) > 0 {
		opts = append(opts, WithManagedPolicies(strings.Join(def.ManagedPolicies, ",")))
	}
	if def.InlinePolicy != "" {
		opts = append(opts, WithInlinePolicy(def.InlinePolicy))
	}
	if def.ResourcePolicy != "" {
		opts = append(opts, WithResourcePolicy(def.ResourcePolicy))
	}
	if def.Timeout > 0 {
		opts = append(opts, WithTimeout(def.Timeout))
	}
	if def.MemorySize > 0 {
		opts = append(opts, WithMemorySize(def.MemorySize))
	}
	if def.Description != "" {
		opts = append(opts, WithDescription(def.Description))
	}
	if def.Environment != nil {
		opts = append(opts, WithEnvironment(def.Environment))
	}
	return opts
}
