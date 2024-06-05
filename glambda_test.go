package glambda_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/smithy-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mr-joshcrane/glambda"
	mock "github.com/mr-joshcrane/glambda/testdata/mock_clients"
)

func init() {
	os.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	glambda.UUID = func() string {
		return "DEADBEEF"
	}
	glambda.AWSAccountID = func(client glambda.STSClient) (string, error) {
		return "123456789012", nil
	}
	glambda.DefaultRetryWaitingPeriod = func() {
		// No need to wait in tests
	}
}

func TestGetAWSAccountID(t *testing.T) {
	t.Parallel()
	client := mock.DummySTSClient{
		AccountID: "123456789012",
	}
	got, err := glambda.GetAWSAccountID(client)
	if err != nil {
		t.Error(err)
	}
	if got != "123456789012" {
		t.Errorf("expected 123456789012, got %s", got)
	}
}

func TestGetAWSAccountID_ErrorCase(t *testing.T) {
	t.Parallel()
	client := mock.DummySTSClient{
		Err: fmt.Errorf("some error"),
	}
	_, err := glambda.GetAWSAccountID(client)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNewLambda(t *testing.T) {
	t.Parallel()
	l, err := glambda.NewLambda("test", "testdata/correct_test_handler/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "test" {
		t.Errorf("expected name to be test, got %s", l.Name)
	}
	if l.HandlerPath != "testdata/correct_test_handler/main.go" {
		t.Errorf("expected handler path to be testdata/correct_test_handler/main.go, got %s", l.HandlerPath)
	}
	want := glambda.ExecutionRole{
		RoleName:                 "glambda_exec_role_test",
		RoleARN:                  "arn:aws:iam::123456789012:role/glambda_exec_role_test",
		AssumeRolePolicyDocument: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`,
		ManagedPolicies:          []string{"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"},
	}
	if !cmp.Equal(want, l.ExecutionRole) {
		t.Error(cmp.Diff(want, l.ExecutionRole))
	}
}

func TestExecutionRole_CreateRoleCommand(t *testing.T) {
	t.Parallel()
	assumePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	roleCmd := glambda.CreateRoleCommand("testRole", assumePolicy)
	want := &iam.CreateRoleInput{
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
	policyARN := "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
	attachCmd := glambda.AttachManagedPolicyCommand("testRole", policyARN)
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
	inlineCmd := glambda.AttachInLinePolicyCommand("testRoleName", "testPolicyName", inlinePolicy)
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

func TestPrepareAction_CreateFunction(t *testing.T) {
	t.Parallel()

	client := mock.DummyLambdaClient{
		FuncExists: false,
		Err:        nil,
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}
	action, err := glambda.PrepareLambdaAction(l, client)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := action.(glambda.LambdaCreateAction)
	if !ok {
		t.Errorf("expected CreateAction, got %T", action)
	}

}

func TestPrepareAction_UpdateFunction(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: true,
		Err:        nil,
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}

	action, err := glambda.PrepareLambdaAction(l, client)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := action.(glambda.LambdaUpdateAction)
	if !ok {
		t.Errorf("expected UpdateAction, got %T", action)
	}
}

func TestPrepareAction_ErrorCase(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: false,
		Err:        fmt.Errorf("some client error"),
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}
	_, err := glambda.PrepareLambdaAction(l, client)
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
	if file.Mode() != 0o755 {
		t.Errorf("expected bootstrap file mode to be 0755, got %d", zipReader.File[0].Mode())
	}
	if file.UncompressedSize64 == 0 {
		t.Errorf("expected bootstrap file to have content, got 0")
	}
}

func TestCreateLambdaCommand(t *testing.T) {
	t.Parallel()
	cmd := glambda.CreateLambdaCommand("lambdaName", "arn:aws:iam::123456789012:role/lambda-role", []byte("some valid zip data"))
	want := &lambda.CreateFunctionInput{
		FunctionName: aws.String("lambdaName"),
		Role:         aws.String("arn:aws:iam::123456789012:role/lambda-role"),
		Code: &types.FunctionCode{
			ZipFile: []byte("some valid zip data"),
		},
		Architectures: []types.Architecture{"arm64"},
		Handler:       aws.String("/var/task/bootstrap"),
		Runtime:       types.RuntimeProvidedal2023,
	}
	ignore := cmpopts.IgnoreUnexported(lambda.CreateFunctionInput{}, types.FunctionCode{})
	if !cmp.Equal(cmd, want, ignore) {
		t.Error(cmp.Diff(cmd, want, ignore))
	}
}

func TestUpdateLambdaCommand(t *testing.T) {
	t.Parallel()
	cmd := glambda.UpdateLambdaCommand("lambdaName", []byte("some valid zip data"))
	want := &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String("lambdaName"),
		ZipFile:      []byte("some valid zip data"),
		Publish:      true,
	}
	ignore := cmpopts.IgnoreUnexported(lambda.UpdateFunctionCodeInput{})
	if !cmp.Equal(cmd, want, ignore) {
		t.Error(cmp.Diff(cmd, want, ignore))
	}
}

func TestPutRolePolicyCommand_WhereCommandExists(t *testing.T) {
	t.Parallel()
	role := glambda.ExecutionRole{
		RoleName:     "aRoleName",
		InLinePolicy: `some inline policy`,
	}
	cmds := glambda.PutRolePolicyCommand(role)
	want := []iam.PutRolePolicyInput{
		{
			PolicyName:     aws.String("glambda_inline_policy_DEADBEEF"),
			PolicyDocument: aws.String(`some inline policy`),
			RoleName:       aws.String("aRoleName"),
		},
	}
	ignore := cmpopts.IgnoreUnexported(iam.PutRolePolicyInput{})
	if !cmp.Equal(cmds, want, ignore) {
		t.Error(cmp.Diff(cmds, want, ignore))
	}
}

func TestPutRolePolicyCommand_WhereCommandDoesNotExist(t *testing.T) {
	t.Parallel()
	role := glambda.ExecutionRole{
		RoleName: "aRoleName",
	}
	cmds := glambda.PutRolePolicyCommand(role)
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands, got %d", len(cmds))
	}
}

func TestPrepareRoleAction_CreatesRoleWhenRoleDoesNotExist(t *testing.T) {
	t.Parallel()
	got, err := glambda.PrepareRoleAction(glambda.ExecutionRole{
		RoleName:                 "aRoleName",
		AssumeRolePolicyDocument: glambda.DefaultAssumeRolePolicy,
	}, mock.DummyIAMClient{
		RoleExists: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := glambda.RoleCreateOrUpdate{
		CreateRole: &iam.CreateRoleInput{
			RoleName:                 aws.String("aRoleName"),
			AssumeRolePolicyDocument: aws.String(glambda.DefaultAssumeRolePolicy),
		},
		ManagedPolicies: []iam.AttachRolePolicyInput{
			{
				PolicyArn: aws.String(glambda.AWSLambdaBasicExecutionRole),
				RoleName:  aws.String("aRoleName"),
			},
		},
	}
	ignore := cmpopts.IgnoreUnexported(iam.CreateRoleInput{}, iam.AttachRolePolicyInput{}, glambda.RoleCreateOrUpdate{})
	if !cmp.Equal(want, got, ignore) {
		t.Error(cmp.Diff(want, got, ignore))
	}
}

func TestPrepareRoleAction_DoesNotCreateRoleWhenRoleExists(t *testing.T) {
	t.Parallel()
	got, err := glambda.PrepareRoleAction(glambda.ExecutionRole{
		RoleName:                 "aRoleName",
		AssumeRolePolicyDocument: glambda.DefaultAssumeRolePolicy,
	}, mock.DummyIAMClient{
		RoleExists: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := glambda.RoleCreateOrUpdate{
		ManagedPolicies: []iam.AttachRolePolicyInput{
			{
				PolicyArn: aws.String(glambda.AWSLambdaBasicExecutionRole),
				RoleName:  aws.String("aRoleName"),
			},
		},
	}
	ignore := cmpopts.IgnoreUnexported(iam.CreateRoleInput{}, iam.AttachRolePolicyInput{}, glambda.RoleCreateOrUpdate{})
	if !cmp.Equal(want, got, ignore) {
		t.Error(cmp.Diff(want, got, ignore))
	}
}

func TestPrepareRoleAction_AttachesMultipleManagedPolicies(t *testing.T) {
	t.Parallel()
	got, err := glambda.PrepareRoleAction(glambda.ExecutionRole{
		RoleName:                 "aRoleName",
		AssumeRolePolicyDocument: glambda.DefaultAssumeRolePolicy,
		ManagedPolicies:          []string{"arn:aws:iam::aws:policy/IAMFullAccess", "arn:aws:iam::aws:policy/AmazonDynamoDBReadOnlyAccess"},
	}, mock.DummyIAMClient{
		RoleExists: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := glambda.RoleCreateOrUpdate{
		CreateRole: &iam.CreateRoleInput{
			RoleName:                 aws.String("aRoleName"),
			AssumeRolePolicyDocument: aws.String(glambda.DefaultAssumeRolePolicy),
		},
		ManagedPolicies: []iam.AttachRolePolicyInput{
			{
				PolicyArn: aws.String(glambda.AWSLambdaBasicExecutionRole),
				RoleName:  aws.String("aRoleName"),
			},
			{
				PolicyArn: aws.String("arn:aws:iam::aws:policy/IAMFullAccess"),
				RoleName:  aws.String("aRoleName"),
			},
			{
				PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonDynamoDBReadOnlyAccess"),
				RoleName:  aws.String("aRoleName"),
			},
		},
	}
	ignore := cmpopts.IgnoreUnexported(iam.CreateRoleInput{}, iam.AttachRolePolicyInput{}, glambda.RoleCreateOrUpdate{})
	if !cmp.Equal(want, got, ignore) {
		t.Error(cmp.Diff(want, got, ignore))
	}
}

func TestWaitForConsistency_PassesForConsistentVersion(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		ConsistantAfterXRetries: aws.Int(8),
	}
	_, err := glambda.WaitForConsistency(client, "testLambda")
	if err != nil {
		t.Error(err)
	}
}

func TestWaitForConsistency_FailsForInconsistentVersion(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{}
	_, err := glambda.WaitForConsistency(client, "testLambda")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestUpdateLambdaActionDo(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: true,
	}
	action := glambda.NewLambdaUpdateAction(client, glambda.Lambda{Name: "testLambda"}, []byte("some valid zip data"))
	err := action.Do()
	if err != nil {
		t.Error(err)
	}
}

func TestCreateLambdaActionDo(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: false,
	}
	l := glambda.Lambda{
		Name:        "testLambda",
		HandlerPath: "testdata/correct_test_handler/main.go",
		ExecutionRole: glambda.ExecutionRole{
			RoleName:                 "lambda-role",
			AssumeRolePolicyDocument: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`,
		},
		ResourcePolicy: glambda.ResourcePolicy{
			Principal:               "123456789012",
			SourceArnCondition:      aws.String("arn:aws:s3:::mybucket"),
			SourceAccountCondition:  aws.String("123456789012"),
			PrincipalOrgIdCondition: aws.String("o-123456"),
		},
	}

	action := glambda.NewLambdaCreateAction(client, l, []byte("some valid zip data"))
	err := action.Do()
	if err != nil {
		t.Error(err)
	}
}

func TestCreateRoleActionDo_IfRoleDoesNotExist(t *testing.T) {
	t.Parallel()
	client := mock.DummyIAMClient{
		RoleExists: false,
	}
	action := glambda.NewRoleCreateOrUpdateAction(client)
	action.CreateRole = &iam.CreateRoleInput{
		RoleName:                 aws.String("aRoleName"),
		AssumeRolePolicyDocument: aws.String(glambda.DefaultAssumeRolePolicy),
	}
	err := action.Do()
	if err != nil {
		t.Error(err)
	}
}

func TestCreateRoleActionDo_FailsIfRoleExists(t *testing.T) {
	t.Parallel()
	client := mock.DummyIAMClient{
		RoleExists: true,
	}
	action := glambda.NewRoleCreateOrUpdateAction(client)
	action.CreateRole = &iam.CreateRoleInput{
		RoleName:                 aws.String("aRoleName"),
		AssumeRolePolicyDocument: aws.String(glambda.DefaultAssumeRolePolicy),
	}
	err := action.Do()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestCreateRoleActionDo_AttachesManagedPolicies(t *testing.T) {
	t.Parallel()
	var clientCallCounter int32
	client := mock.DummyIAMClient{
		RoleExists: false,
		Counter:    &clientCallCounter,
	}
	action := glambda.NewRoleCreateOrUpdateAction(client)
	action.CreateRole = &iam.CreateRoleInput{
		RoleName:                 aws.String("aRoleName"),
		AssumeRolePolicyDocument: aws.String(glambda.DefaultAssumeRolePolicy),
	}
	action.ManagedPolicies = []iam.AttachRolePolicyInput{
		{
			PolicyArn: aws.String(glambda.AWSLambdaBasicExecutionRole),
			RoleName:  aws.String("aRoleName"),
		},
		{
			PolicyArn: aws.String("arn:aws:iam::aws:policy/AmazonDynamoDBReadOnlyAccess"),
			RoleName:  aws.String("aRoleName"),
		},
	}
	action.InlinePolicies = []iam.PutRolePolicyInput{
		{
			PolicyName:     aws.String("glambda_inline_policy_DEADBEEF"),
			PolicyDocument: aws.String(`some inline policy`),
		},
	}
	err := action.Do()
	if err != nil {
		t.Error(err)
	}
	if clientCallCounter != 4 {
		t.Errorf("expected 4 client calls, got %d", clientCallCounter)
	}
}

func TestRetryableErrors_OperationalErrorsAreRetried(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description string
		err         error
		want        aws.Ternary
	}{
		{
			description: "ResourceNotFoundException",
			err: &smithy.OperationError{
				ServiceID:     "lambda",
				OperationName: "GetFunction",
				Err: &types.ResourceNotFoundException{
					Message: aws.String("Resource not found"),
					Type:    aws.String("ResourceNotFoundException"),
				},
			},
			want: aws.FalseTernary,
		},
		{
			description: "InvalidParameterValueException",
			err: &smithy.OperationError{
				ServiceID:     "lambda",
				OperationName: "GetFunction",
				Err: &types.InvalidParameterValueException{
					Message: aws.String("The role defined for the function cannot be assumed by Lambda"),
					Type:    aws.String("InvalidParameterValueException"),
				},
			},
			want: aws.TrueTernary,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			r := glambda.RetryableErrors{}
			got := r.IsErrorRetryable(tc.err)
			if got != tc.want {
				t.Errorf("for %s: expected %v, got %v", tc.description, tc.want, got)
			}
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	t.Parallel()
	got := glambda.GenerateUUID()
	// 8 alphanumeric characters, no dashes, underscores or capitals
	criteria := regexp.MustCompile(`^[a-z0-9]{8}$`)
	if !criteria.MatchString(got) {
		t.Errorf("expected 8 alphanumeric characters, got %s", got)
	}
}

func TestCreateLambdaResourcePolicy_NoConditions(t *testing.T) {
	t.Parallel()
	l := glambda.Lambda{
		Name: "testLambda",
		ResourcePolicy: glambda.ResourcePolicy{
			Principal: "123456789012",
		},
	}
	got := l.CreateLambdaResourcePolicy()
	want := &lambda.AddPermissionInput{
		Action:       aws.String("lambda:InvokeFunction"),
		FunctionName: aws.String("testLambda"),
		Principal:    aws.String("123456789012"),
		StatementId:  aws.String("glambda_invoke_permission_DEADBEEF"),
	}
	ignore := cmpopts.IgnoreUnexported(lambda.AddPermissionInput{})
	if !cmp.Equal(got, want, ignore) {
		t.Error(cmp.Diff(got, want, ignore))
	}
}

func TestCreateLambdaResourcePolicy_WithConditions(t *testing.T) {
	t.Parallel()
	l := glambda.Lambda{
		Name: "testLambda",
		ResourcePolicy: glambda.ResourcePolicy{
			Principal:               "s3.amazonaws.com",
			SourceAccountCondition:  aws.String("123456789012"),
			SourceArnCondition:      aws.String("arn:aws:s3:::mybucket"),
			PrincipalOrgIdCondition: aws.String("o-123456"),
		},
	}
	got := l.CreateLambdaResourcePolicy()
	want := &lambda.AddPermissionInput{
		Action:         aws.String("lambda:InvokeFunction"),
		FunctionName:   aws.String("testLambda"),
		Principal:      aws.String("s3.amazonaws.com"),
		StatementId:    aws.String("glambda_invoke_permission_DEADBEEF"),
		SourceAccount:  aws.String("123456789012"),
		SourceArn:      aws.String("arn:aws:s3:::mybucket"),
		PrincipalOrgID: aws.String("o-123456"),
	}
	ignore := cmpopts.IgnoreUnexported(lambda.AddPermissionInput{})
	if !cmp.Equal(got, want, ignore) {
		t.Error(cmp.Diff(got, want, ignore))
	}
}

func TestWithManagedPolicies_ParsesMessyUserInputIntoExecutionManagePolicies(t *testing.T) {
	t.Parallel()
	l := glambda.Lambda{
		Name: "testLambda",
		ExecutionRole: glambda.ExecutionRole{
			RoleName: "aRoleName",
		},
	}
	opt := glambda.WithManagedPolicies(`
			"arn:aws:iam::aws:policy/IAMFullAccess",
			arn:aws:iam::aws:policy/AmazonDynamoDBReadOnlyAccess,
			'AmazonS3ReadOnlyAccess'
		 `)
	err := opt(&l)
	if err != nil {
		t.Error(err)
	}
	want := []string{
		"arn:aws:iam::aws:policy/IAMFullAccess",
		"arn:aws:iam::aws:policy/AmazonDynamoDBReadOnlyAccess",
		"arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess",
	}
	got := l.ExecutionRole.ManagedPolicies
	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(got, want))
	}
}

func TestWithInlinePolicy_ParsesMessyUserInputIntoExecutionInlinePolicy(t *testing.T) {
	t.Parallel()
	l := glambda.Lambda{
		Name: "testLambda",
		ExecutionRole: glambda.ExecutionRole{
			RoleName: "aRoleName",
		},
	}
	policy := `
		{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": "logs:CreateLogGroup",
					"Resource": "arn:aws:logs:us-west-2:123456789012:*"
				},
				{
					"Effect": "Allow",
					"Action": "logs:CreateLogStream",
					"Resource": "arn:aws:logs:us-west-2:123456789012:log-group:/aws/lambda/test:*"
				}
			]
		}`

	opt := glambda.WithInlinePolicy(policy)
	err := opt(&l)
	if err != nil {
		t.Error(err)
	}
	want := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, policy)
	got := l.ExecutionRole.InLinePolicy
	if !cmp.Equal(want, got) {
		t.Error(cmp.Diff(want, got))
	}
}

func TestWithInlinePolicy_CanDetectInvalidPolicyCases(t *testing.T) {
	testCases := []struct {
		description string
		policy      string
	}{
		{
			description: "empty policy",
			policy:      "",
		},
		{
			description: "invalid json",
			policy:      `{"invalid": "json}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			l := glambda.Lambda{}
			opt := glambda.WithInlinePolicy(tc.policy)
			err := opt(&l)
			if err == nil {
				t.Errorf("%s, expected error, got nil", tc.description)
			}
		})
	}
}
