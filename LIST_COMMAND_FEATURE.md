# List Command Feature

## Overview
Added a new `list` command to glambda that shows all Lambda functions previously deployed by glambda.

## Implementation

### Identification Strategy
- **Tag-based identification**: All Lambda functions created by glambda now have a `ManagedBy: glambda` tag
- This allows reliable identification of glambda-managed functions vs. other Lambda functions

### Changes Made

#### 1. Core Library (`glambda.go`)
- **Modified `CreateLambdaCommand()`**: Added `Tags` field with `"ManagedBy": "glambda"` to all created functions
- **Added `LambdaInfo` struct**: Represents Lambda function information (Name, Runtime, LastModified, Role)
- **Added `ListGlambdaFunctions(client LambdaClient)`**: Core function that lists glambda-managed Lambdas (testable)
- **Added `List()`**: Convenience function that creates a client and calls `ListGlambdaFunctions()`

#### 2. Client Interface (`plumbing.go`)
- **Modified `LambdaClient` interface**: Added `ListFunctions()` method

#### 3. CLI Command (`command/command.go`)
- **Added `ListCommand()`**: New CLI command that displays glambda-managed functions in a formatted table
- **Output format**: Displays NAME, RUNTIME, and LAST MODIFIED in columns
- **Updated `Main()`**: Registered the new list command

#### 4. Mock Client (`testdata/mock_clients/mock_clients.go`)
- **Enhanced `DummyLambdaClient`**: Added `Functions` and `FunctionTags` fields
- **Modified `GetFunction()`**: Returns tags when `FunctionTags` is populated
- **Added `ListFunctions()`**: Returns mock function list for testing

#### 5. Tests (`glambda_test.go`)
- **Updated `TestCreateLambdaCommand`**: Verifies the `ManagedBy` tag is included
- **Added `TestListGlambdaFunctions_FiltersGlambdaManagedFunctions`**: Tests filtering logic
- **Added `TestListGlambdaFunctions_ReturnsEmptyListWhenNoGlambdaFunctions`**: Tests empty results

## Usage

```bash
# List all glambda-managed Lambda functions
glambda list
```

### Example Output
```
NAME                           RUNTIME              LAST MODIFIED                 
--------------------------------------------------------------------------------
my-function                    provided.al2023      2024-01-01T00:00:00.000+0000  
another-function               provided.al2023      2024-01-02T10:30:00.000+0000  
```

If no glambda-managed functions exist:
```
No glambda-managed Lambda functions found.
```

## Implementation Details

### How it Works
1. Calls AWS Lambda `ListFunctions` API to get all functions in the region
2. For each function, calls `GetFunction` to retrieve tags
3. Filters functions that have `ManagedBy: glambda` tag
4. Returns function metadata (name, runtime, last modified, role)

### Design Decisions

**Why tags?**
- Reliable: Tags persist with the Lambda function
- Non-intrusive: Doesn't interfere with function naming or other metadata
- AWS-native: Uses standard AWS tagging functionality
- Backward compatible: Existing functions without tags won't be listed (intentional)

**Why not role name pattern?**
- Glambda creates roles with pattern `glambda_exec_role_*`, but users might customize roles
- Tags are more explicit and reliable

**Testability**
- Separated `ListGlambdaFunctions(client)` from `List()` to allow dependency injection
- Follows existing codebase patterns (see `GetAWSAccountID`)

## Backward Compatibility

- **Existing deployments**: Functions deployed before this feature won't have the tag and won't appear in `glambda list`
- **Future deployments**: All new deployments will be tagged automatically
- **Workaround for existing**: Re-deploy existing functions with `glambda deploy` to add the tag

## Testing

All tests pass:
```bash
go test -v
```

Key tests:
- `TestCreateLambdaCommand`: Verifies tag is added during creation
- `TestListGlambdaFunctions_FiltersGlambdaManagedFunctions`: Verifies filtering logic
- `TestListGlambdaFunctions_ReturnsEmptyListWhenNoGlambdaFunctions`: Verifies empty case

## Files Modified

- `glambda.go` - Core list functionality and tag addition
- `plumbing.go` - LambdaClient interface update
- `command/command.go` - CLI command
- `glambda_test.go` - Tests
- `testdata/mock_clients/mock_clients.go` - Mock client enhancements
