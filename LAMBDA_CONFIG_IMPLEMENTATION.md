# Lambda Configuration Options - Implementation Complete

## Summary

Successfully implemented support for common Lambda configuration options (Timeout, MemorySize, Environment variables, Description) for both creating new and updating existing Lambda functions.

## Features Implemented

### 1. Core Configuration Support
- **LambdaConfig struct** added to Lambda with pointers for optional values
- **Timeout** (int32) - execution timeout in seconds (1-900)
- **MemorySize** (int32) - memory allocation in MB (128-10240)
- **Environment** (map[string]string) - environment variables
- **Description** (string) - function description

### 2. Functional Options Pattern
Following the existing `DeployOptions` pattern:
- `WithTimeout(timeout int)` - sets function timeout
- `WithMemorySize(memory int)` - sets function memory
- `WithEnvironment(env map[string]string)` - sets environment variables
- `WithDescription(desc string)` - sets function description

### 3. CLI Flags
Added to `glambda deploy` command:
- `--timeout` - Function timeout in seconds (1-900)
- `--memory-size` - Function memory in MB (128-10240)
- `--environment` - Environment variables as KEY1=VAL1,KEY2=VAL2
- `--description` - Function description

### 4. Environment Variable Parsing
- `ParseEnvironment()` helper function for CLI
- Format: `KEY1=VAL1,KEY2=VAL2`
- Handles spaces, values with equals signs
- Comprehensive validation and error messages

### 5. Create and Update Support
**For new Lambdas:**
- `CreateLambdaCommand()` applies configuration during creation

**For existing Lambdas:**
- `UpdateConfigurationCommand()` updates configuration
- `LambdaUpdateAction` now updates both code and configuration
- Returns nil if no config options are set (optimization)

## Usage Examples

### Basic deployment with timeout and memory
```bash
glambda deploy myFunction handler.go \
  --timeout 30 \
  --memory-size 512
```

### With environment variables
```bash
glambda deploy myFunction handler.go \
  --environment "DB_HOST=localhost,DB_PORT=5432,ENV=production" \
  --description "Production API handler"
```

### Combined with policies
```bash
glambda deploy myFunction handler.go \
  --timeout 60 \
  --memory-size 1024 \
  --environment "TABLE_NAME=users" \
  --managed-policies "arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess" \
  --description "DynamoDB handler with 60s timeout"
```

## Implementation Details

### Architecture
1. **LambdaConfig** struct holds all configuration options
2. **Functional options** modify Lambda.Config before deployment
3. **CreateLambdaCommand** applies config during Lambda creation
4. **UpdateConfigurationCommand** applies config during Lambda updates
5. **CLI flags** parse user input and create functional options

### Key Design Decisions

**Pointers for optional values:**
- Using `*int32` and `*string` allows distinguishing "not set" from "set to default"
- AWS SDK requires pointers for optional fields

**Separate code and config updates:**
- `UpdateFunctionCode` and `UpdateFunctionConfiguration` are separate AWS API calls
- Must be called sequentially (code first, then config)
- Only calls UpdateFunctionConfiguration if config options are set

**Environment variable format:**
- Chose `KEY=VAL,KEY2=VAL2` over JSON for CLI simplicity
- Handles edge cases: spaces, values with equals signs
- Clear error messages for invalid format

### Code Quality

**Follows existing patterns:**
- ✅ No nested conditionals
- ✅ Single path through functions  
- ✅ Functional options pattern like `WithManagedPolicies`
- ✅ Command helpers like `CreateLambdaCommand`
- ✅ Comprehensive tests for all functionality

**Test Coverage:**
- Configuration struct tests
- Functional options tests (WithTimeout, WithMemorySize, etc.)
- CreateLambdaCommand with config
- UpdateConfigurationCommand (with and without config)
- Environment variable parsing (valid and invalid cases)
- All tests passing ✅

## Files Modified

### Core Library
- `glambda.go` - Added LambdaConfig, With* options, Update*Command
- `plumbing.go` - Added UpdateFunctionConfiguration to interface

### CLI
- `command/command.go` - Added flags and ParseEnvironment helper

### Tests
- `glambda_test.go` - Tests for config options and commands
- `command/command_test.go` - Tests for ParseEnvironment

### Mocks
- `testdata/mock_clients/mock_clients.go` - Added UpdateFunctionConfiguration

## Commits

1. `d9e20d7` - Add Lambda configuration options support
   - Core LambdaConfig struct and functional options
   - CLI flags and environment parsing
   - Tests for create operations

2. `5f3a107` - Add configuration update support for existing Lambdas
   - UpdateConfigurationCommand helper
   - Enhanced LambdaUpdateAction
   - Tests for update operations

## Future Enhancements (Not Implemented)

The following Lambda configuration options were considered but not implemented:
- **EphemeralStorage** - /tmp directory size (default 512 MB)
- **Layers** - Lambda layers for shared code
- **VPC Configuration** - Subnet and security group settings
- **Reserved Concurrency** - Concurrent execution limits
- **Dead Letter Config** - DLQ for failed invocations

These can be added following the same pattern if needed.

## Testing

All tests passing:
```bash
$ go test ./...
ok      github.com/mr-joshcrane/glambda         2.439s
ok      github.com/mr-joshcrane/glambda/command 2.281s
```

## Example Test Run

```bash
# Create with configuration
glambda deploy testFunc handler.go --timeout 45 --memory-size 256

# Update existing function with new config
glambda deploy testFunc handler.go --timeout 60 --memory-size 512 \
  --environment "ENV=prod,DEBUG=false"

# List deployed functions
glambda list
```
