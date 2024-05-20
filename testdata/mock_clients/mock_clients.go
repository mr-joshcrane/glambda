package mock

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type DummyLambdaClient struct {
	ConsistantAfterXRetries *int
	FuncExists              bool
	Err                     error
}

func (d DummyLambdaClient) GetFunction(ctx context.Context, input *lambda.GetFunctionInput, opts ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	if d.FuncExists {
		return &lambda.GetFunctionOutput{}, nil
	}
	if !d.FuncExists && d.Err == nil {
		return &lambda.GetFunctionOutput{}, new(types.ResourceNotFoundException)
	}
	if d.Err != nil {
		return &lambda.GetFunctionOutput{}, d.Err
	}
	return &lambda.GetFunctionOutput{}, d.Err
}

func (d DummyLambdaClient) CreateFunction(ctx context.Context, input *lambda.CreateFunctionInput, opts ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error) {
	return &lambda.CreateFunctionOutput{}, nil
}

func (d DummyLambdaClient) UpdateFunctionCode(ctx context.Context, input *lambda.UpdateFunctionCodeInput, opts ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error) {
	return &lambda.UpdateFunctionCodeOutput{}, d.Err
}

func (d DummyLambdaClient) Invoke(ctx context.Context, input *lambda.InvokeInput, opts ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
	return &lambda.InvokeOutput{
		StatusCode: 200,
		Payload:    []byte("all good"),
	}, nil
}

func (d DummyLambdaClient) PublishVersion(ctx context.Context, input *lambda.PublishVersionInput, opts ...func(*lambda.Options)) (*lambda.PublishVersionOutput, error) {
	if d.ConsistantAfterXRetries == nil {
		return &lambda.PublishVersionOutput{}, fmt.Errorf("this lambda never becomes consistent")
	}
	if *d.ConsistantAfterXRetries > 0 {
		*d.ConsistantAfterXRetries--
		return &lambda.PublishVersionOutput{}, fmt.Errorf("not yet consistent")
	}
	return &lambda.PublishVersionOutput{
		Version: aws.String("1"),
	}, nil
}

func (d DummyLambdaClient) AddPermission(ctx context.Context, input *lambda.AddPermissionInput, opts ...func(*lambda.Options)) (*lambda.AddPermissionOutput, error) {
	return &lambda.AddPermissionOutput{}, nil
}

func (d DummyLambdaClient) DeleteFunction(ctx context.Context, input *lambda.DeleteFunctionInput, opts ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
	return &lambda.DeleteFunctionOutput{}, nil
}

type DummyIAMClient struct {
	RoleExists bool
	RoleName   string
	Counter    *int32
}

func (d DummyIAMClient) IncrementCounter() {
	if d.Counter != nil {
		atomic.AddInt32(d.Counter, 1)
	}
}

func (d DummyIAMClient) CreateRole(ctx context.Context, input *iam.CreateRoleInput, opts ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	d.IncrementCounter()
	if d.RoleExists {
		return &iam.CreateRoleOutput{}, new(iTypes.EntityAlreadyExistsException)
	}
	return &iam.CreateRoleOutput{}, nil
}

func (d DummyIAMClient) AttachRolePolicy(ctx context.Context, input *iam.AttachRolePolicyInput, opts ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	d.IncrementCounter()
	return &iam.AttachRolePolicyOutput{}, nil
}

func (d DummyIAMClient) PutRolePolicy(ctx context.Context, input *iam.PutRolePolicyInput, opts ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error) {
	d.IncrementCounter()
	return &iam.PutRolePolicyOutput{}, nil
}

func (d DummyIAMClient) GetRole(ctx context.Context, input *iam.GetRoleInput, opts ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	d.IncrementCounter()
	if d.RoleExists {
		return &iam.GetRoleOutput{
			Role: &iTypes.Role{
				RoleName: aws.String(d.RoleName),
			},
		}, nil
	}
	return &iam.GetRoleOutput{}, new(iTypes.NoSuchEntityException)
}

type DummySTSClient struct {
	AccountID string
	Err       error
}

func (d DummySTSClient) GetCallerIdentity(ctx context.Context, input *sts.GetCallerIdentityInput, opts ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	return &sts.GetCallerIdentityOutput{
		Account: aws.String(d.AccountID),
	}, nil

}
