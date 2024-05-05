package glambda

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/google/uuid"
)

var (
	DefaultAssumeRolePolicy     = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	AWSLambdaBasicExecutionRole = `arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole`
	ThisAWSAccountCondition     = `"Condition":{"StringEquals":{"aws:PrincipalAccount": "${aws:accountId}"}}"`
)

type Lambda struct {
	Name           string
	HandlerPath    string
	ExecutionRole  ExecutionRole
	ResourcePolicy ResourcePolicy
	cfg            *aws.Config
}
type LambdaOptions func(l Lambda) Lambda

func NewLambda(name, handlerPath string, opts ...LambdaOptions) Lambda {
	l := Lambda{
		Name:        name,
		HandlerPath: handlerPath,
		ExecutionRole: ExecutionRole{
			RoleName:                 "glambda_exec_role_" + strings.ToLower(name),
			AssumeRolePolicyDocument: DefaultAssumeRolePolicy,
		},
	}
	for _, opt := range opts {
		l = opt(l)
	}
	return l
}

type ExecutionRole struct {
	RoleName                 string
	AssumeRolePolicyDocument string
	ManagedPolicies          []string
	InLinePolicy             string
}

func (e ExecutionRole) CreateRoleCommand() iam.CreateRoleInput {
	return iam.CreateRoleInput{
		RoleName:                 aws.String(e.RoleName),
		AssumeRolePolicyDocument: aws.String(e.AssumeRolePolicyDocument),
	}
}

func (e ExecutionRole) AttachManagedPolicyCommand(policyARN string) iam.AttachRolePolicyInput {
	return iam.AttachRolePolicyInput{
		PolicyArn: aws.String(policyARN),
		RoleName:  aws.String(e.RoleName),
	}
}

func (e ExecutionRole) AttachInLinePolicyCommand(policyName string) iam.PutRolePolicyInput {
	return iam.PutRolePolicyInput{
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(e.InLinePolicy),
		RoleName:       aws.String(e.RoleName),
	}
}

type ResourcePolicy struct {
	Sid       string
	Effect    string
	Principal string
	Action    string
	Resource  string
	Condition string
}

func (r ResourcePolicy) CreateCommand(lambdaName, accountID string) lambda.AddPermissionInput {
	if r.Sid == "" {
		uuid := uuid.New().String()[0:8]
		r.Sid = "glambda_" + uuid
	}
	return lambda.AddPermissionInput{
		Action:        aws.String(r.Action),
		FunctionName:  aws.String(lambdaName),
		StatementId:   aws.String(r.Sid),
		Principal:     aws.String(r.Principal),
		SourceAccount: aws.String(accountID),
	}
}

func WithExecutionRole(name string, opts ...RoleOptions) LambdaOptions {
	executionRole := ExecutionRole{
		RoleName:                 name,
		AssumeRolePolicyDocument: DefaultAssumeRolePolicy,
		ManagedPolicies: []string{
			"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
		},
	}
	for _, opt := range opts {
		opt(&executionRole)
	}
	return func(l Lambda) Lambda {
		l.ExecutionRole = executionRole
		return l
	}
}

type RoleOptions func(role *ExecutionRole)

func expandManagedPolicies(policyARNs []string) []string {
	var expandedPolicyArns []string
	for _, policyARN := range policyARNs {
		if strings.HasPrefix(policyARN, "arn:") {
			expandedPolicyArns = append(expandedPolicyArns, policyARN)
		} else {
			expandedPolicyArns = append(expandedPolicyArns, "arn:aws:iam::aws:policy/"+policyARN)
		}
	}
	return expandedPolicyArns
}

func WithManagedPolicies(policyARNs ...string) RoleOptions {
	return func(role *ExecutionRole) {
		role.ManagedPolicies = append(role.ManagedPolicies, expandManagedPolicies(policyARNs)...)
	}
}

func WithInlinePolicy(policy string) RoleOptions {
	return func(role *ExecutionRole) {
		role.InLinePolicy = policy
	}
}

func WithResourcePolicy(serviceName string) LambdaOptions {
	resourcePolicy := ResourcePolicy{
		Effect:    "Allow",
		Principal: serviceName,
		Action:    "lambda:InvokeFunction",
		Resource:  "*",
		Condition: ThisAWSAccountCondition,
	}
	return func(l Lambda) Lambda {
		l.ResourcePolicy = resourcePolicy
		return l
	}
}

func WithAWSConfig(cfg aws.Config) LambdaOptions {
	return func(l Lambda) Lambda {
		l.cfg = &cfg
		return l
	}
}

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
	var err error
	cmd := createLambda(a.Name, a.Role, a.Pkg)
	for i := 0; i < 3; i++ {
		_, err = c.CreateFunction(context.Background(), &cmd)
		if err == nil {
			fmt.Printf("Lambda function %s created\n", a.Name)
			return nil
		}
		if errors.Is(err, &types.InvalidParameterValueException{}) {
			invalidParamErr := err.(*types.InvalidParameterValueException)
			if !strings.Contains(*invalidParamErr.Message, "role defined for the function cannot be assumed by Lambda") {
				return err
			}
			time.Sleep(3 * time.Second)
		}
	}
	return err
}

type UpdateAction struct {
	Name string
	Pkg  []byte
}

func (a UpdateAction) Do(c LambdaClient) error {
	cmd := updateLambda(a.Name, a.Pkg)
	_, err := c.UpdateFunctionCode(context.Background(), &cmd)
	if err == nil {
		fmt.Printf("Lambda function %s updated\n", a.Name)
	}
	return err
}

func (l Lambda) Deploy() error {
	if l.cfg == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return err
		}
		l.cfg = &cfg
	}
	roleARN, err := l.PrepareExecutionRole()
	if err != nil {
		return err
	}
	fmt.Println("Created execution role: ", roleARN)
	c := lambda.NewFromConfig(*l.cfg)
	action, err := PrepareAction(c, l.Name, l.HandlerPath, roleARN)
	if err != nil {
		return err
	}
	err = action.Do(c)
	if err != nil {
		return err
	}
	accountID := strings.Split(roleARN, ":")[4]
	resourcePolicy := l.ResourcePolicy.CreateCommand(l.Name, accountID)
	_, err = c.AddPermission(context.Background(), &resourcePolicy)
	if err != nil {
		return err
	}
	return invokeUpdatedLambda(c, l.Name)
}

func (l Lambda) PrepareExecutionRole() (string, error) {
	if l.cfg == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return "", err
		}
		l.cfg = &cfg
	}
	c := iam.NewFromConfig(*l.cfg)
	var roleARN string
	resp, err := c.CreateRole(context.Background(), &iam.CreateRoleInput{
		RoleName:                 aws.String(l.ExecutionRole.RoleName),
		AssumeRolePolicyDocument: aws.String(l.ExecutionRole.AssumeRolePolicyDocument),
	})
	if err == nil {
		roleARN = *resp.Role.Arn
	}
	if err != nil {
		var i *iTypes.EntityAlreadyExistsException
		if errors.As(err, &i) {
			resp, err := c.GetRole(context.Background(), &iam.GetRoleInput{
				RoleName: aws.String(l.ExecutionRole.RoleName),
			})
			if err != nil {
				return "", err
			}
			roleARN = *resp.Role.Arn
		}
	}
	_, err = c.AttachRolePolicy(context.Background(), &iam.AttachRolePolicyInput{
		PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		RoleName:  aws.String(l.ExecutionRole.RoleName),
	})
	for _, managedPolicy := range l.ExecutionRole.ManagedPolicies {
		_, err = c.AttachRolePolicy(context.Background(), &iam.AttachRolePolicyInput{
			PolicyArn: aws.String(managedPolicy),
			RoleName:  aws.String(l.ExecutionRole.RoleName),
		})
		if err != nil {
			return "", err
		}
	}
	uuid := uuid.New().String()[0:8]
	guid := strings.ReplaceAll(uuid, "-", "")
	if l.ExecutionRole.InLinePolicy != "" {
		_, err = c.PutRolePolicy(context.Background(), &iam.PutRolePolicyInput{
			PolicyName:     aws.String("glambda_inline_policy_" + guid[:8]),
			PolicyDocument: aws.String(l.ExecutionRole.InLinePolicy),
			RoleName:       aws.String(l.ExecutionRole.RoleName),
		})
		if err != nil {
			return "", err
		}
	}
	return roleARN, err
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
	retryLimit := 10
	fmt.Println("Waiting for lambda to become eventually consistent before invoking")
	for i := 0; true; i++ {
		versionOutput, err := c.PublishVersion(context.Background(), &lambda.PublishVersionInput{
			FunctionName: aws.String(name),
		})
		if err == nil {
			version = *versionOutput.Version
			fmt.Printf("Lambda is consistent! Lambda version published: %s\n", version)
			break
		}
		time.Sleep(3 * time.Second)
		if i == retryLimit {
			return fmt.Errorf("waited for lambda become consistent, but didn't after %d retries, %w", retryLimit, err)
		}
	}

	resp, err := c.Invoke(context.Background(), &lambda.InvokeInput{
		FunctionName: aws.String(name),
		Qualifier:    aws.String(version),
	})
	if err != nil {
		return err
	}
	fmt.Println("Invocation result:")
	if resp.FunctionError != nil {
		fmt.Println("Error:")
		fmt.Println(*resp.FunctionError)
	}
	if resp.Payload != nil {
		fmt.Println("Payload:")
		fmt.Println(string(resp.Payload))
	}
	if resp.LogResult != nil {
		fmt.Println("Logs:")
		fmt.Println(string(*resp.LogResult))
	}
	fmt.Println(resp.StatusCode)
	return nil
}
