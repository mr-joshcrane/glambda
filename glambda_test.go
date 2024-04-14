package glambda_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/google/go-cmp/cmp"
	"github.com/mr-joshcrane/glambda"
)

func TestPrepareAction_CreateFunction(t *testing.T) {
	t.Parallel()
	client := helperDummyLambdaClient(false, nil)
	handler := "testdata/correct_test_handler.go"
	action, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err != nil {
		t.Error(err)
	}
	got, ok := action.(glambda.CreateAction)
	if !ok {
		t.Errorf("expected CreateAction, got %T", action)
	}
	if got.Name != "test" {
		t.Errorf("expected name to be test, got %s", got.Name)
	}
}

func TestPrepareAction_UpdateFunction(t *testing.T) {
	t.Parallel()
	client := helperDummyLambdaClient(true, nil)
	handler := "testdata/correct_test_handler.go"
	action, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err != nil {
		t.Error(err)
	}
	got, ok := action.(glambda.UpdateAction)
	if !ok {
		t.Errorf("expected UpdateAction, got %T", action)
	}
	if got.Name != "test" {
		t.Errorf("expected name to be test, got %s", got.Name)
	}
}

func TestPrepareAction_ErrorCase(t *testing.T) {
	t.Parallel()
	client := helperDummyLambdaClient(false, fmt.Errorf("some client error"))
	handler := "testdata/correct_test_handler.go"
	_, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestValidate_AcceptsCorrectlySetupLambdaSourceFile(t *testing.T) {
	t.Parallel()
	handler := "testdata/correct_test_handler.go"
	err := glambda.Validate(handler)
	if err != nil {
		t.Error(err)
	}
}

func TestValidate_RejectsIncorrectlySetupLambdaSourceFiles(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description string
		filename    string
	}{
		{
			description: "invalid handler function",
			filename:    "testdata/missing_handler.go",
		},
		{
			description: "missing main function",
			filename:    "testdata/missing_main.go",
		},
		{
			description: "missing lambda.Start(handler) call",
			filename:    "testdata/missing_lambda_start.go",
		},
		{
			description: "invalid go source file",
			filename:    "testdata/invalid_go_source.go",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := glambda.Validate(tc.filename)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestPackage_PackagesLambdaFunction(t *testing.T) {
	t.Parallel()
	handler := "testdata/correct_test_handler.go"
	data, err := glambda.Package(handler)
	if err != nil {
		t.Error(err)
	}
	want, err := os.ReadFile("testdata/correct_test_handler.zip")
	if err != nil {
		t.Errorf("failed to read in test data, %v", err)
	}
	if !cmp.Equal(data, want) {
		t.Errorf("zip file data doesn't match: expected length %d, got %d", len(want), len(data))
	}
}

type DummyLambdaClient struct {
	funcExists bool
	err        error
}

func (d DummyLambdaClient) GetFunction(ctx context.Context, input *lambda.GetFunctionInput, opts ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	if d.funcExists {
		return &lambda.GetFunctionOutput{}, nil
	}
	if !d.funcExists && d.err == nil {
		return &lambda.GetFunctionOutput{}, new(types.ResourceNotFoundException)
	}
	if d.err != nil {
		return &lambda.GetFunctionOutput{}, d.err
	}
	return &lambda.GetFunctionOutput{}, d.err
}

func (d DummyLambdaClient) CreateFunction(ctx context.Context, input *lambda.CreateFunctionInput, opts ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error) {
	return &lambda.CreateFunctionOutput{}, nil
}

func (d DummyLambdaClient) UpdateFunctionCode(ctx context.Context, input *lambda.UpdateFunctionCodeInput, opts ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error) {
	return &lambda.UpdateFunctionCodeOutput{}, nil
}

func helperDummyLambdaClient(exists bool, err error) glambda.LambdaClient {
	return DummyLambdaClient{
		funcExists: exists,
		err:        err,
	}
}
