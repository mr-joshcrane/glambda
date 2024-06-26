package glambda

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"
)

// LambdaClient represents the interface that a lambda client should implement.
//
// The most obvious implementation is the lambda.Client from the aws-sdk-go-v2
// However we also use it for mock clients in tests
type LambdaClient interface {
	CreateFunction(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error)
	UpdateFunctionCode(ctx context.Context, params *lambda.UpdateFunctionCodeInput, optFns ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error)
	GetFunction(ctx context.Context, params *lambda.GetFunctionInput, optFns ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
	PublishVersion(ctx context.Context, params *lambda.PublishVersionInput, optFns ...func(*lambda.Options)) (*lambda.PublishVersionOutput, error)
	Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
	AddPermission(ctx context.Context, params *lambda.AddPermissionInput, optFns ...func(*lambda.Options)) (*lambda.AddPermissionOutput, error)
	DeleteFunction(ctx context.Context, params *lambda.DeleteFunctionInput, optFns ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
}

// IAMClient represents the interface that an iam client should implement.
//
// The most obvious implementation is the iam.Client from the aws-sdk-go-v2
// However we also use it for mock clients in tests
type IAMClient interface {
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	PutRolePolicy(ctx context.Context, params *iam.PutRolePolicyInput, optFns ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
}

// STSClient represents the interface that an sts client should implement.
//
// The most obvious implementation is the sts.Client from the aws-sdk-go-v2
// However we also use it for mock clients in tests
type STSClient interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// CreateRoleCommand is a paperwork reducer that translates parameters into
// the smithy autogenerated AWS IAM SDKv2 format of [iam.CreateRoleInput]
func CreateRoleCommand(roleName string, assumePolicyDocument string) *iam.CreateRoleInput {
	return &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumePolicyDocument),
	}
}

// GetRoleCommand is a paperwork reducer that translates parameters into
// the smithy autogenerated AWS IAM SDKv2 format of [iam.GetRoleInput]
func AttachManagedPolicyCommand(roleName string, policyARN string) iam.AttachRolePolicyInput {
	return iam.AttachRolePolicyInput{
		RoleName:  aws.String(roleName),
		PolicyArn: aws.String(policyARN),
	}
}

// AttachInLinePolicyCommand is a paperwork reducer that translates parameters into
// the smithy autogenerated AWS IAM SDKv2 format of [iam.PutRolePolicyInput]
func AttachInLinePolicyCommand(roleName string, policyName string, inlinePolicy string) iam.PutRolePolicyInput {
	return iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(inlinePolicy),
	}
}

// CreateLambdaResourcePolicy is a paperwork reducer that takes the definition
// of a lambda and creates an appropriate [lambda.AddPermissionInput] payload.
// This payload is sent to the AWS API to allow the source defined in the lambda.ResourcePolicy
// to invoke this lambda. This is useful for example when you want to allow only a particular
// AWS Service or AWS Principal (ie. Account, Role, User) to invoke the lambda.
func (l Lambda) CreateLambdaResourcePolicy() *lambda.AddPermissionInput {
	if l.ResourcePolicy.Principal == "" {
		return nil
	}
	return &lambda.AddPermissionInput{
		Action:         aws.String("lambda:InvokeFunction"),
		FunctionName:   aws.String(l.Name),
		StatementId:    aws.String("glambda_invoke_permission_" + UUID()),
		Principal:      aws.String(l.ResourcePolicy.Principal),
		SourceAccount:  l.ResourcePolicy.SourceAccountCondition,
		SourceArn:      l.ResourcePolicy.SourceArnCondition,
		PrincipalOrgID: l.ResourcePolicy.PrincipalOrgIdCondition,
	}
}

// PutRolePolicyCommand is a paperwork reducer that takes the definition of an
// execution role and creates an appropriate [iam.PutRolePolicyInput] payload.
// This payload is sent to the AWS API to attach an inline policy to a given
// AWS IAM Role. Useful for when you need to give fine grained permissions to your
// Lambda Execution Role
func PutRolePolicyCommand(role ExecutionRole) []iam.PutRolePolicyInput {
	var inputs []iam.PutRolePolicyInput
	if role.InLinePolicy == "" {
		return inputs
	}
	cmd := iam.PutRolePolicyInput{
		PolicyName:     aws.String("glambda_inline_policy_" + UUID()),
		PolicyDocument: aws.String(role.InLinePolicy),
		RoleName:       aws.String(role.RoleName),
	}
	inputs = append(inputs, cmd)
	return inputs
}

var (
	DefaultAssumeRolePolicy     = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	AWSLambdaBasicExecutionRole = `arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole`
	ThisAWSAccountCondition     = `"Condition":{"StringEquals":{"aws:PrincipalAccount": "${aws:accountId}"}}"`
)

var UUID = GenerateUUID
var AWSAccountID = GetAWSAccountID

func GenerateUUID() string {
	id := uuid.New().String()
	id = strings.ReplaceAll(id, ":", "")
	return id[0:8]
}

// GetAWSAccountID calls the AWS STS API to get the user credentials that the user
// is using to make the API call. This response contains the AWS Account ID of the IAM Principal
func GetAWSAccountID(client STSClient) (string, error) {
	resp, err := client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return *resp.Account, nil
}

var DefaultRetryWaitingPeriod = func() {
	time.Sleep(3 * time.Second)
}

// WaitForConsistence deals with the fact that lambda functions are eventually consistent.
// Deploying a lambda function and then immediately invoking it can result in an invocation
// of the previous version of the lambda function, which could mask deployment failures.
// This function waits for the lambda function to become consistent by publishing a new version
// which seems to wait on the backend until the lambda function is consistent.
func WaitForConsistency(c LambdaClient, name string) (string, error) {
	retryLimit := 10
	for i := 0; true; i++ {
		resp, err := c.PublishVersion(context.Background(), &lambda.PublishVersionInput{
			FunctionName: aws.String(name),
		})
		if err == nil {
			if resp.Version == nil {
				return "", fmt.Errorf("version is nil")
			}
			return *resp.Version, nil
		}
		DefaultRetryWaitingPeriod()
		if i == retryLimit {
			break
		}
	}
	return "", fmt.Errorf("waited for lambda become consistent, but didn't after %d retries", retryLimit)
}

func lambdaExists(c LambdaClient, name string) (bool, error) {
	input := &lambda.GetFunctionInput{
		FunctionName: aws.String(name),
	}
	_, err := c.GetFunction(context.Background(), input)
	if err != nil {
		var resourceNotFound *types.ResourceNotFoundException
		if errors.As(err, &resourceNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func customRetryer() aws.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = 20
		o.Retryables = append(o.Retryables, RetryableErrors{})
	})
}

type RetryableErrors struct{}

// IsErrorRetryable is a custom retryer that tells the lambda client
// to retry on which errors.
func (r RetryableErrors) IsErrorRetryable(err error) aws.Ternary {
	var lambdaErr *types.InvalidParameterValueException
	if errors.As(err, &lambdaErr) {
		return aws.TrueTernary
	}
	return aws.FalseTernary
}
