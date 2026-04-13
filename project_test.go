package glambda_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-joshcrane/glambda"
)

func TestParseConfig_ValidConfig(t *testing.T) {
	t.Parallel()
	config, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "processOrders"
handler = "./cmd/process/main.go"
timeout = 30
memory-size = 256
description = "Processes incoming orders"
managed-policies = ["AmazonSESFullAccess"]
inline-policy = '{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}'

[lambda.environment]
DB_HOST = "mydb.example.com"

[[lambda]]
name = "sendNotifications"
handler = "./cmd/notify/main.go"
timeout = 10
`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Project.Name != "my-service" {
		t.Errorf("expected project name 'my-service', got %s", config.Project.Name)
	}
	if len(config.Lambda) != 2 {
		t.Fatalf("expected 2 lambdas, got %d", len(config.Lambda))
	}
	first := config.Lambda[0]
	if first.Name != "processOrders" {
		t.Errorf("expected name 'processOrders', got %s", first.Name)
	}
	if first.Handler != "./cmd/process/main.go" {
		t.Errorf("expected handler './cmd/process/main.go', got %s", first.Handler)
	}
	if first.Timeout != 30 {
		t.Errorf("expected timeout 30, got %d", first.Timeout)
	}
	if first.MemorySize != 256 {
		t.Errorf("expected memory-size 256, got %d", first.MemorySize)
	}
	if first.Description != "Processes incoming orders" {
		t.Errorf("expected description 'Processes incoming orders', got %s", first.Description)
	}
	if len(first.ManagedPolicies) != 1 || first.ManagedPolicies[0] != "AmazonSESFullAccess" {
		t.Errorf("expected managed-policies [AmazonSESFullAccess], got %v", first.ManagedPolicies)
	}
	if first.Environment["DB_HOST"] != "mydb.example.com" {
		t.Errorf("expected DB_HOST=mydb.example.com, got %s", first.Environment["DB_HOST"])
	}

	second := config.Lambda[1]
	if second.Name != "sendNotifications" {
		t.Errorf("expected name 'sendNotifications', got %s", second.Name)
	}
	if second.Timeout != 10 {
		t.Errorf("expected timeout 10, got %d", second.Timeout)
	}
}

func TestParseConfig_MissingProjectName(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`
[[lambda]]
name = "test"
handler = "./main.go"
`)
	if err == nil {
		t.Error("expected error for missing project name")
	}
}

func TestParseConfig_NoLambdas(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"
`)
	if err == nil {
		t.Error("expected error for no lambda definitions")
	}
}

func TestParseConfig_MissingLambdaName(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
handler = "./main.go"
`)
	if err == nil {
		t.Error("expected error for missing lambda name")
	}
}

func TestParseConfig_MissingHandler(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
`)
	if err == nil {
		t.Error("expected error for missing handler")
	}
}

func TestParseConfig_DuplicateLambdaNames(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
handler = "./a.go"

[[lambda]]
name = "test"
handler = "./b.go"
`)
	if err == nil {
		t.Error("expected error for duplicate lambda names")
	}
}

func TestParseConfig_InvalidTOML(t *testing.T) {
	t.Parallel()
	_, err := glambda.ParseConfig(`this is not valid toml {{{`)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "glambda.toml")
	content := `
[project]
name = "file-test"

[[lambda]]
name = "myFunc"
handler = "./main.go"
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	config, err := glambda.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Project.Name != "file-test" {
		t.Errorf("expected project name 'file-test', got %s", config.Project.Name)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := glambda.LoadConfig("/nonexistent/glambda.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestConfigHash_Deterministic(t *testing.T) {
	t.Parallel()
	def := glambda.LambdaDefinition{
		Name:       "test",
		Handler:    "./main.go",
		Timeout:    30,
		MemorySize: 256,
		Environment: map[string]string{
			"B": "2",
			"A": "1",
		},
		ManagedPolicies: []string{"PolicyB", "PolicyA"},
	}
	hash1 := glambda.ConfigHash(def)
	hash2 := glambda.ConfigHash(def)
	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %s and %s", hash1, hash2)
	}
	if len(hash1) != 16 {
		t.Errorf("expected 16 char hash, got %d chars", len(hash1))
	}
}

func TestConfigHash_DiffersOnChange(t *testing.T) {
	t.Parallel()
	def1 := glambda.LambdaDefinition{
		Name:    "test",
		Handler: "./main.go",
		Timeout: 30,
	}
	def2 := glambda.LambdaDefinition{
		Name:    "test",
		Handler: "./main.go",
		Timeout: 60,
	}
	if glambda.ConfigHash(def1) == glambda.ConfigHash(def2) {
		t.Error("expected different hashes for different configs")
	}
}

func TestToDeployOptions_AllFields(t *testing.T) {
	t.Parallel()
	def := glambda.LambdaDefinition{
		Name:            "test",
		Handler:         "./main.go",
		Timeout:         30,
		MemorySize:      256,
		Description:     "A test function",
		ManagedPolicies: []string{"AmazonS3ReadOnlyAccess"},
		InlinePolicy:    `{"Effect":"Allow"}`,
		ResourcePolicy:  `{"Principal":{"Service":"s3.amazonaws.com"},"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"arn:aws:lambda:us-east-1:123456789012:function:test"}`,
		Environment:     map[string]string{"KEY": "VAL"},
	}
	opts := def.ToDeployOptions()
	// 7 options: managed policies, inline policy, resource policy, timeout, memory, description, environment
	if len(opts) != 7 {
		t.Errorf("expected 7 deploy options, got %d", len(opts))
	}
}

func TestToDeployOptions_MinimalFields(t *testing.T) {
	t.Parallel()
	def := glambda.LambdaDefinition{
		Name:    "test",
		Handler: "./main.go",
	}
	opts := def.ToDeployOptions()
	if len(opts) != 0 {
		t.Errorf("expected 0 deploy options for minimal config, got %d", len(opts))
	}
}

func TestParseConfig_ResolvesEnvVarReferences(t *testing.T) {
	t.Setenv("TEST_SECRET_KEY", "super-secret-value")
	config, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
handler = "./main.go"

[lambda.environment]
API_KEY = "${TEST_SECRET_KEY}"
STATIC_VAL = "plaintext"
`)
	if err != nil {
		t.Fatal(err)
	}
	env := config.Lambda[0].Environment
	if env["API_KEY"] != "super-secret-value" {
		t.Errorf("expected resolved value 'super-secret-value', got %q", env["API_KEY"])
	}
	if env["STATIC_VAL"] != "plaintext" {
		t.Errorf("expected static value 'plaintext', got %q", env["STATIC_VAL"])
	}
}

func TestParseConfig_FailsOnMissingEnvVar(t *testing.T) {
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
handler = "./main.go"

[lambda.environment]
SECRET = "${DEFINITELY_NOT_SET_ANYWHERE}"
`)
	if err == nil {
		t.Fatal("expected error for unset env var reference")
	}
	if !strings.Contains(err.Error(), "DEFINITELY_NOT_SET_ANYWHERE") {
		t.Errorf("expected error to mention the missing var, got: %s", err)
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("expected error to say 'not set', got: %s", err)
	}
}

func TestParseConfig_FailsOnEmptyEnvVar(t *testing.T) {
	t.Setenv("EMPTY_SECRET", "")
	_, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
handler = "./main.go"

[lambda.environment]
SECRET = "${EMPTY_SECRET}"
`)
	if err == nil {
		t.Fatal("expected error for empty env var")
	}
}

func TestParseConfig_MultiplerefsInOneValue(t *testing.T) {
	t.Setenv("HOST", "db.example.com")
	t.Setenv("PORT", "5432")
	config, err := glambda.ParseConfig(`
[project]
name = "my-service"

[[lambda]]
name = "test"
handler = "./main.go"

[lambda.environment]
DB_URL = "${HOST}:${PORT}"
`)
	if err != nil {
		t.Fatal(err)
	}
	if config.Lambda[0].Environment["DB_URL"] != "db.example.com:5432" {
		t.Errorf("expected 'db.example.com:5432', got %q", config.Lambda[0].Environment["DB_URL"])
	}
}
