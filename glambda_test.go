package glambda_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/mr-joshcrane/glambda"
)

func TestPrepareAction_CreateFunction(t *testing.T) {
	t.Parallel()
	client := helperDummyLambdaClient(false, nil)
	handler := "testdata/correct_test_handler/main.go"
	action, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err != nil {
		t.Fatal(err)
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
	handler := "testdata/correct_test_handler/main.go"
	action, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err != nil {
		t.Fatal(err)
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
	handler := "testdata/correct_test_handler/main.go"
	_, err := glambda.PrepareAction(client, "test", handler, "arn:aws:iam::123456789012:role/lambda-role")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestValidate_AcceptsCorrectlySetupLambdaSourceFile(t *testing.T) {
	t.Parallel()
	handler := "testdata/correct_test_handler/main.go"
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
		{
			description: "invalid handler signature",
			filename:    "testdata/invalid_handler_signature.go",
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
	handler := "testdata/correct_test_handler/main.go"
	data, err := glambda.Package(handler)
	if err != nil {
		t.Error(err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("failed to create zip reader, %v", err)
	}
	if len(zipReader.File) != 1 {
		t.Errorf("expected 1 file in zip, got %d", len(zipReader.File))
	}
	file := zipReader.File[0]
	if file.Name != "bootstrap" {
		t.Errorf("expected file name to be bootstrap, got %s", zipReader.File[0].Name)
	}
	if file.Mode() != 0755 {
		t.Errorf("expected bootstrap file mode to be 0755, got %d", zipReader.File[0].Mode())
	}
	if file.UncompressedSize64 == 0 {
		t.Errorf("expected bootstrap file to have content, got 0")
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
