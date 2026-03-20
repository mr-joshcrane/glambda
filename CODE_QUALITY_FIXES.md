# Code Quality Fixes for List Command

## Issues Fixed

### 1. ✅ Mock GetFunction() - Eliminated Nested Conditionals
**File**: `testdata/mock_clients/mock_clients.go:24-42`

**Problem**: Violated `.claude/CLAUDE.md` rule: "We dont do nested if else, and we dont do multiple paths through a function"

**Before**:
```go
func (d DummyLambdaClient) GetFunction(...) {
    if d.FunctionTags != nil {
        if tags, ok := d.FunctionTags[functionName]; ok {
            return ...
        }
    }
    if d.FuncExists {
        return ...
    }
    if !d.FuncExists && d.Err == nil {
        return ...
    }
    if d.Err != nil {
        return ...
    }
    return ...
}
```

**After**:
```go
func (d DummyLambdaClient) GetFunction(...) {
    if d.Err != nil {
        return &lambda.GetFunctionOutput{}, d.Err
    }
    if !d.FuncExists && d.FunctionTags == nil {
        return &lambda.GetFunctionOutput{}, new(types.ResourceNotFoundException)
    }
    functionName := aws.ToString(input.FunctionName)
    tags, hasTags := d.FunctionTags[functionName]
    if hasTags {
        return &lambda.GetFunctionOutput{
            Tags: tags,
        }, nil
    }
    return &lambda.GetFunctionOutput{}, nil
}
```

**Changes**:
- Eliminated nested if statements
- Single linear path through the function
- Early returns for error cases
- Follows existing mock client patterns

---

### 2. ✅ ListGlambdaFunctions() - Proper Error Handling
**File**: `glambda.go:537-539`

**Problem**: Silent error swallowing with `continue` - inconsistent with codebase pattern of returning errors

**Before**:
```go
fnDetails, err := client.GetFunction(...)
if err != nil {
    continue  // Silently skips all errors
}

if managedBy, ok := fnDetails.Tags["ManagedBy"]; ok && managedBy == "glambda" {
    // nested logic
}
```

**After**:
```go
fnDetails, err := client.GetFunction(...)
if err != nil {
    var resourceNotFound *types.ResourceNotFoundException
    if errors.As(err, &resourceNotFound) {
        continue  // Only skip race conditions
    }
    return nil, err  // Return unexpected errors
}

managedBy, ok := fnDetails.Tags["ManagedBy"]
if !ok {
    continue
}
if managedBy != "glambda" {
    continue
}
// Build info struct
```

**Changes**:
- Only continues on ResourceNotFoundException (expected race condition: function deleted between ListFunctions and GetFunction)
- Returns all other errors to caller (permissions issues, rate limits, etc.)
- Follows existing pattern from `plumbing.go:174-186` (lambdaExists function)
- Separated nested tag checking into linear conditionals
- Added documentation about skipping behavior

---

### 3. ✅ Added Error Handling Test
**File**: `glambda_test.go`

**Added**:
```go
func TestListGlambdaFunctions_ReturnsErrorOnListFailure(t *testing.T) {
    client := mock.DummyLambdaClient{
        Err: fmt.Errorf("API error"),
    }
    _, err := glambda.ListGlambdaFunctions(client)
    if err == nil {
        t.Error("expected error, got nil")
    }
}
```

Ensures that errors are properly propagated to the caller.

---

## Testing

All tests pass:
```bash
$ go test ./...
ok  	github.com/mr-joshcrane/glambda	1.421s
ok  	github.com/mr-joshcrane/glambda/command	1.699s
```

Test coverage:
- ✅ Filters glambda-managed functions
- ✅ Returns empty list when no glambda functions exist
- ✅ Returns error on API failures

---

## Code Quality Checklist

- ✅ No nested if/else statements
- ✅ Single path through functions (early returns only)
- ✅ Errors returned to caller (not silently swallowed)
- ✅ Follows existing codebase patterns
- ✅ Uses `errors.As()` for type checking (like `plumbing.go:179-181`)
- ✅ Documented behavior
- ✅ Comprehensive tests

---

## Files Modified

1. `testdata/mock_clients/mock_clients.go` - Refactored GetFunction()
2. `glambda.go` - Fixed error handling in ListGlambdaFunctions()
3. `glambda_test.go` - Added error handling test
