package glambda_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mr-joshcrane/glambda"
)

func TestNewLambda(t *testing.T) {
	t.Parallel()
	l := glambda.NewLambda("test", "testdata/correct_test_handler/main.go")
	if l.Name != "test" {
		t.Errorf("expected name to be test, got %s", l.Name)
	}
	if l.HandlerPath != "testdata/correct_test_handler/main.go" {
		t.Errorf("expected handler path to be testdata/correct_test_handler/main.go, got %s", l.HandlerPath)
	}
	if !cmp.Equal(l.ExecutionRole, glambda.ExecutionRole{}) {
		t.Errorf("expected execution role to be empty, got %v", l.ExecutionRole)
	}
	if !cmp.Equal(l.ResourcePolicy, glambda.ResourcePolicy{}) {
		t.Errorf("expected resource policy to be empty, got %v", l.ResourcePolicy)
	}
}

func TestNewLambda_WithExecutionRole(t *testing.T) {
	t.Parallel()
	execRole := glambda.ExecutionRole{
		RoleName: "arn:aws:iam::123456789012:role/lambda-role",
	}
	l := glambda.NewLambda("test", "testdata/correct_test_handler/main.go", glambda.WithExecutionRole(execRole))
	if !cmp.Equal(l.ExecutionRole, execRole) {
		t.Error(cmp.Diff(l.ExecutionRole, execRole))
	}
}

func TestNewLambda_WithResourcePolicy(t *testing.T) {
	t.Parallel()
	resourcePolicy := glambda.ResourcePolicy{
		Sid:       "AllowExecutionFromS3",
		Effect:    "Allow",
		Principal: "s3.amazonaws.com",
		Action:    "lambda:InvokeFunction",
		Resource:  "arn:aws:lambda:us-west-2:123456789012:function:test",
		Condition: "",
	}
	l := glambda.NewLambda("test", "testdata/correct_test_handler/main.go", glambda.WithResourcePolicy(resourcePolicy))

	if !cmp.Equal(l.ResourcePolicy, resourcePolicy) {
		t.Error(cmp.Diff(l.ResourcePolicy, resourcePolicy))
	}
}

func TestExecutionRole_CreateRoleCommand(t *testing.T) {
	t.Parallel()
	assumePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	execRole := glambda.ExecutionRole{
		RoleName:                 "testRole",
		AssumeRolePolicyDocument: assumePolicy,
	}
	roleCmd := execRole.CreateRoleCommand()
	want := iam.CreateRoleInput{
		RoleName:                 aws.String("testRole"),
		AssumeRolePolicyDocument: aws.String(assumePolicy),
	}
	ignore := cmpopts.IgnoreUnexported(iam.CreateRoleInput{})
	if !cmp.Equal(roleCmd, want, ignore) {
		t.Error(cmp.Diff(roleCmd, want, ignore))
	}
}

func TestExecutionRole_AttachManagedPolicyCommand(t *testing.T) {
	t.Parallel()
	execRole := glambda.ExecutionRole{
		RoleName: "testRole",
	}
	policyARN := "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
	attachCmd := execRole.AttachManagedPolicyCommand(policyARN)
	want := iam.AttachRolePolicyInput{
		PolicyArn: aws.String(policyARN),
		RoleName:  aws.String("testRole"),
	}
	ignore := cmpopts.IgnoreUnexported(iam.AttachRolePolicyInput{})
	if !cmp.Equal(attachCmd, want, ignore) {
		t.Error(cmp.Diff(attachCmd, want, ignore))
	}
}

func TestExecutionRole_AttachInLinePolicyCommand(t *testing.T) {
	t.Parallel()
	inlinePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"logs:CreateLogGroup","Resource":"arn:aws:logs:us-west-2:123456789012:*"},{"Effect":"Allow","Action":"logs:CreateLogStream","Resource":"arn:aws:logs:us-west-2:123456789012:log-group:/aws/lambda/test:*"}]}`
	execRole := glambda.ExecutionRole{
		RoleName:     "testRoleName",
		InLinePolicy: inlinePolicy,
	}
	inlineCmd := execRole.AttachInLinePolicyCommand("testPolicyName")
	want := iam.PutRolePolicyInput{
		PolicyName:     aws.String("testPolicyName"),
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"logs:CreateLogGroup","Resource":"arn:aws:logs:us-west-2:123456789012:*"},{"Effect":"Allow","Action":"logs:CreateLogStream","Resource":"arn:aws:logs:us-west-2:123456789012:log-group:/aws/lambda/test:*"}]}`),
		RoleName:       aws.String("testRoleName"),
	}
	ignore := cmpopts.IgnoreUnexported(iam.PutRolePolicyInput{})
	if !cmp.Equal(inlineCmd, want, ignore) {
		t.Error(cmp.Diff(inlineCmd, want, ignore))
	}
}

func TestResourcePolicy_ToAddPermissionCommand(t *testing.T) {
	t.Parallel()
	resourcePolicy := glambda.ResourcePolicy{
		Sid:       "AllowExecutionFromS3",
		Effect:    "Allow",
		Principal: "s3.amazonaws.com",
		Action:    "lambda:InvokeFunction",
		Resource:  "arn:aws:lambda:us-west-2:123456789012:function:test",
		Condition: "",
	}
	rpCmd := resourcePolicy.CreateCommand("testName")
	want := lambda.AddPermissionInput{
		Action:        &resourcePolicy.Action,
		FunctionName:  aws.String("testName"),
		StatementId:   &resourcePolicy.Sid,
		Principal:     &resourcePolicy.Principal,
		SourceArn:     &resourcePolicy.Resource,
		SourceAccount: &resourcePolicy.Condition,
	}
	ignore := cmpopts.IgnoreUnexported(lambda.AddPermissionInput{})
	if !cmp.Equal(rpCmd, want, ignore) {
		t.Error(cmp.Diff(rpCmd, want, ignore))
	}
}

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
