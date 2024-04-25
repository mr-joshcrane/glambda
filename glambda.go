package glambda

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type LambdaClient interface {
	CreateFunction(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error)
	UpdateFunctionCode(ctx context.Context, params *lambda.UpdateFunctionCodeInput, optFns ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error)
	GetFunction(ctx context.Context, params *lambda.GetFunctionInput, optFns ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
}

type Action interface {
	Do(c LambdaClient) error
}

type CreateAction struct {
	Name string
	Role string
	Pkg  []byte
}

func (a CreateAction) Do(c LambdaClient) error {
	cmd := createLambda(a.Name, a.Role, a.Pkg)
	_, err := c.CreateFunction(context.Background(), &cmd)
	return err
}

type UpdateAction struct {
	Name string
	Pkg  []byte
}

func (a UpdateAction) Do(c LambdaClient) error {
	cmd := updateLambda(a.Name, a.Pkg)
	_, err := c.UpdateFunctionCode(context.Background(), &cmd)
	return err
}

type Options func(*aws.Config) error

func WithExecutionRolePermissions(roleName, policy string) Options {
	fmt.Println(roleName)
	return func(cfg *aws.Config) error {
		c := iam.NewFromConfig(*cfg)
		_, err := c.PutRolePolicy(context.Background(), &iam.PutRolePolicyInput{
			RoleName:       aws.String(roleName),
			PolicyName:     aws.String("glambda_permissions"),
			PolicyDocument: aws.String(policy),
		})
		return err
	}
}

func WithLambdaResourcePolicy(lambdaName, identifier, principal, sourceARN, sourceAccount string) Options {
	return func(cfg *aws.Config) error {
		c := lambda.NewFromConfig(*cfg)
		_, err := c.AddPermission(context.Background(), &lambda.AddPermissionInput{
			Action:        aws.String("lambda:InvokeFunction"),
			FunctionName:  aws.String(lambdaName),
			StatementId:   aws.String(identifier),
			Principal:     aws.String(principal),
			SourceArn:     aws.String(sourceARN),
			SourceAccount: aws.String(sourceAccount),
		})
		return err
	}
}

func Deploy(name, path string, opts ...Options) error {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return err
	}

	roleARN, err := PrepareExecutionRole(cfg)
	if err != nil {
		return err
	}
	c := lambda.NewFromConfig(cfg)
	action, err := PrepareAction(c, name, path, roleARN)
	if err != nil {
		return err
	}
	err = action.Do(c)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		err := opt(&cfg)
		if err != nil {
			return err
		}
	}
	return invokeUpdatedLambda(c, name)
}

func PrepareExecutionRole(cfg aws.Config) (string, error) {
	c := iam.NewFromConfig(cfg)
	resp, err := c.CreateRole(context.Background(), &iam.CreateRoleInput{
		RoleName:                 aws.String("glambda_execution_role"),
		AssumeRolePolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
	})
	if err != nil {
		var i *iTypes.EntityAlreadyExistsException
		if errors.As(err, &i) {
			resp, err := c.GetRole(context.Background(), &iam.GetRoleInput{
				RoleName: aws.String("glambda_execution_role"),
			})
			if err != nil {
				return "", err
			}
			return *resp.Role.Arn, nil
		}
		return "", err
	}
	_, err = c.AttachRolePolicy(context.Background(), &iam.AttachRolePolicyInput{
		PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		RoleName:  aws.String("glambda_execution_role"),
	})

	return *resp.Role.Arn, err
}

func PrepareAction(c LambdaClient, name, path, roleARN string) (Action, error) {
	exists, err := lambdaExists(c, name)
	if err != nil {
		return nil, err
	}
	pkg, err := Package(path)
	if err != nil {
		return nil, err
	}
	var action Action
	if exists {
		action = UpdateAction{
			Name: name,
			Pkg:  pkg,
		}
	} else {
		action = CreateAction{
			Name: name,
			Role: roleARN,
			Pkg:  pkg,
		}
	}
	return action, nil
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

func createLambda(name, role string, pkg []byte) lambda.CreateFunctionInput {
	return lambda.CreateFunctionInput{
		FunctionName: aws.String(name),
		Role:         aws.String(role),
		Handler:      aws.String("/var/task/bootstrap"),
		Runtime:      types.RuntimeProvidedal2023,
		Architectures: []types.Architecture{
			types.ArchitectureArm64,
		},
		Code: &types.FunctionCode{
			ZipFile: pkg,
		},
	}
}

func updateLambda(name string, pkg []byte) lambda.UpdateFunctionCodeInput {
	return lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(name),
		ZipFile:      pkg,
		Publish:      true,
	}
}

func invokeUpdatedLambda(c *lambda.Client, name string) error {
	var version string
	for {
		versionOutput, err := c.PublishVersion(context.Background(), &lambda.PublishVersionInput{
			FunctionName: aws.String(name),
		})
		if err == nil {
			version = *versionOutput.Version
			break
		}
		time.Sleep(3 * time.Second)
		fmt.Println("retrying")
	}

	resp, err := c.Invoke(context.Background(), &lambda.InvokeInput{
		FunctionName: aws.String(name),
		Qualifier:    aws.String(version),
	})
	if err != nil {
		return err
	}
	fmt.Println(string(resp.Payload))
	fmt.Println(resp.StatusCode)
	return nil
}
