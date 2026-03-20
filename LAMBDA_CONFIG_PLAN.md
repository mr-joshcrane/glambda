# Lambda Configuration Options - Implementation Plan

## Overview
Add support for common Lambda configuration options (Timeout, MemorySize, Environment variables, Description) to glambda.

## Current State
- `glambda` currently hardcodes Lambda configuration in `CreateLambdaCommand()` (glambda.go:308-321)
- Only Runtime, Architecture, and Handler are set
- No support for Timeout, MemorySize, Environment variables, or other Lambda configuration options
- Uses a functional options pattern (`DeployOptions`) for policies but not for Lambda config

## Available AWS Lambda Configuration Options
- `Timeout` (int32) - execution timeout in seconds
- `MemorySize` (int32) - memory allocation in MB
- `Environment` - environment variables
- `Description` - function description
- `EphemeralStorage` - /tmp directory size
- `Layers` - Lambda layers

## Decisions Made
1. **Scope**: Common options (Timeout, MemorySize, Environment, Description)
2. **API Design**: CLI flags (--timeout, --memory-size, etc.)
3. **Update Behavior**: Support configuration updates for existing Lambdas

## Implementation Plan

### 1. Add LambdaConfig struct (glambda.go)
Create a new struct to hold common configuration options:
```go
type LambdaConfig struct {
    Timeout     *int32
    MemorySize  *int32
    Environment map[string]string
    Description *string
}
```
- Add this field to the `Lambda` struct
- Use pointers to distinguish between "not set" and "set to default"

### 2. Add WithLambdaConfig DeployOptions (glambda.go)
Follow the existing functional options pattern:
```go
func WithTimeout(timeout int) DeployOptions
func WithMemorySize(memory int) DeployOptions
func WithEnvironment(env map[string]string) DeployOptions
func WithDescription(desc string) DeployOptions
```

### 3. Update CreateLambdaCommand (glambda.go:308)
- Modify signature: `CreateLambdaCommand(name, roleARN string, pkg []byte, config LambdaConfig)`
- Add fields to CreateFunctionInput:
  - `Timeout`
  - `MemorySize`
  - `Environment` (convert map to `*types.Environment`)
  - `Description`

### 4. Add UpdateFunctionConfigurationAction (glambda.go)
Create new action for configuration updates:
```go
type LambdaConfigUpdateAction struct {
    client                      LambdaClient
    UpdateConfigurationCommand  *lambda.UpdateFunctionConfigurationInput
}
```
- Use AWS Lambda's `UpdateFunctionConfiguration` API
- Execute this action when updating existing Lambdas

### 5. Update PrepareLambdaAction (glambda.go:282)
When updating existing Lambda, need to handle:
- Code update (existing behavior)
- Configuration update (new behavior)
Consider: Should we check if config actually changed before updating?

### 6. Update CLI flags (command/command.go:60)
Add flags to DeployCommand:
```go
deployCmd.Flags().Int("timeout", 3, "Function timeout in seconds (1-900)")
deployCmd.Flags().Int("memory-size", 128, "Function memory in MB (128-10240)")
deployCmd.Flags().String("environment", "", "Environment variables as KEY1=VAL1,KEY2=VAL2")
deployCmd.Flags().String("description", "", "Function description")
```

### 7. Update tests (glambda_test.go)
- Update `TestCreateLambdaCommand` to verify new fields
- Add tests for:
  - `WithTimeout`, `WithMemorySize`, `WithEnvironment`, `WithDescription`
  - Configuration updates
  - Environment variable parsing

### 8. Update README.md
Add documentation section for configuration options with examples:
```bash
glambda deploy myFunction handler.go \
  --timeout 30 \
  --memory-size 512 \
  --environment "DB_HOST=localhost,DB_PORT=5432" \
  --description "My Lambda function"
```

## Open Questions

1. **Default values**: Should we use AWS defaults (Timeout=3s, Memory=128MB) or different defaults?
   - Current thinking: Use AWS defaults for consistency

2. **Environment variable format**: For the `--environment` flag:
   - Option A: `KEY1=VAL1,KEY2=VAL2` (simpler for shell)
   - Option B: `'{"KEY1":"VAL1"}'` (JSON, more explicit)
   - Current thinking: Option A for CLI convenience

3. **Validation**: Should we validate ranges?
   - Timeout: 1-900 seconds
   - Memory: 128-10240 MB (must be in 1 MB increments, or 64 MB increments for some values)
   - Current thinking: Let AWS validate and return errors

4. **Update behavior**: When updating a Lambda:
   - Should we always update configuration even if unchanged?
   - Should we fetch current config and only update if different?
   - Current thinking: Always update (simpler, idempotent)

## Implementation Order

1. Add `LambdaConfig` struct to `Lambda` struct
2. Add `With*` DeployOptions functions
3. Update `CreateLambdaCommand` to use config
4. Update `NewLambda` to initialize empty config
5. Add CLI flags and wire them to DeployOptions
6. Update tests for create path
7. Add `UpdateFunctionConfigurationAction` for updates
8. Update `PrepareLambdaAction` to handle config updates
9. Add tests for update path
10. Update README

## Files to Modify

- `glambda.go` - Core logic
- `command/command.go` - CLI interface
- `glambda_test.go` - Tests
- `plumbing.go` - May need to add `UpdateFunctionConfiguration` to `LambdaClient` interface
- `testdata/mock_clients/mock_clients.go` - Mock for tests
- `README.md` - Documentation

## Related Code Patterns

Reference existing patterns:
- Functional options: `WithManagedPolicies`, `WithInlinePolicy` (glambda.go:341-380)
- Action pattern: `LambdaCreateAction`, `LambdaUpdateAction` (glambda.go:112-178)
- Command builders: `CreateLambdaCommand`, `UpdateLambdaCommand` (glambda.go:308-331)
