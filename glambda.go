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
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"
)

var (
	DefaultAssumeRolePolicy     = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	AWSLambdaBasicExecutionRole = `arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole`
	ThisAWSAccountCondition     = `"Condition":{"StringEquals":{"aws:PrincipalAccount": "${aws:accountId}"}}"`
)

var UUID = func() string {
	id := uuid.New().String()
	id = strings.ReplaceAll(id, ":", "")
	return id[0:8]
}

var AWSAccountID = func(cfg aws.Config) (string, error) {
	stsClient := sts.NewFromConfig(cfg)
	resp, err := stsClient.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return *resp.Account, nil
}

var DefaultRetryWaitingPeriod = func() {
	time.Sleep(3 * time.Second)
}

type Lambda struct {
	Name           string
	HandlerPath    string
	ExecutionRole  ExecutionRole
	ResourcePolicy ResourcePolicy
	AWSAccountID   string
	cfg            *aws.Config
}
type LambdaOptions func(l Lambda) Lambda

func NewLambda(name, handlerPath string, opts ...LambdaOptions) (Lambda, error) {
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
	if l.cfg == nil {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return Lambda{}, err
		}
		l.cfg = &cfg
	}
	if l.AWSAccountID == "" {
		accountID, err := AWSAccountID(*l.cfg)
		if err != nil {
			return Lambda{}, err
		}
		l.AWSAccountID = accountID
	}
	return l, nil
}

type ExecutionRole struct {
	RoleName                 string
	RoleARN                  string
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
		r.Sid = "glambda_" + UUID()
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

type IAMClient interface {
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	PutRolePolicy(ctx context.Context, params *iam.PutRolePolicyInput, optFns ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
}

type Action interface {
	Do() error
}

type LambdaAction interface {
	Client() LambdaClient
	Action
}

type LambdaCreateAction struct {
	client LambdaClient
	Name   string
	Role   string
	Pkg    []byte
}

func (a LambdaCreateAction) Client() LambdaClient {
	return a.client
}

func (a LambdaCreateAction) Do() error {
	var err error
	client := a.Client()
	cmd := CreateLambdaCommand(a.Name, a.Role, a.Pkg)
	for i := 0; i < 3; i++ {
		_, err = client.CreateFunction(context.Background(), &cmd)
		if err == nil {
			fmt.Printf("Lambda function %s created\n", a.Name)
			return nil
		}
		if errors.Is(err, &types.InvalidParameterValueException{}) {
			invalidParamErr := err.(*types.InvalidParameterValueException)
			if !strings.Contains(*invalidParamErr.Message, "role defined for the function cannot be assumed by Lambda") {
				return err
			}
			DefaultRetryWaitingPeriod()
		}
	}
	return err
}

type LambdaUpdateAction struct {
	client LambdaClient
	Name   string
	Pkg    []byte
}

func (a LambdaUpdateAction) Client() LambdaClient {
	return a.client
}

func (a LambdaUpdateAction) Do() error {
	client := a.Client()
	cmd := UpdateLambdaCommand(a.Name, a.Pkg)
	_, err := client.UpdateFunctionCode(context.Background(), &cmd)
	if err == nil {
		fmt.Printf("Lambda function %s updated\n", a.Name)
	}
	return err
}

func (l Lambda) Deploy() error {
	iamClient := iam.NewFromConfig(*l.cfg)
	roleAction, err := l.PrepareRoleAction(iamClient)
	if err != nil {
		return err
	}
	err = roleAction.Do()
	if err != nil {
		return err
	}
	c := lambda.NewFromConfig(*l.cfg)
	action, err := l.PrepareLambdaAction(c)
	if err != nil {
		return err
	}
	err = action.Do()
	if err != nil {
		return err
	}
	resourcePolicy := l.ResourcePolicy.CreateCommand(l.Name, l.AWSAccountID)
	_, err = c.AddPermission(context.Background(), &resourcePolicy)
	if err != nil {
		return err
	}
	return invokeUpdatedLambda(c, l.Name)
}

type RoleAction interface {
	Client() IAMClient
	Do() error
}

type RoleCreateOrUpdate struct {
	client          IAMClient
	CreateRole      iam.CreateRoleInput
	ManagedPolicies []iam.AttachRolePolicyInput
	InlinePolicies  []iam.PutRolePolicyInput
}

func (a RoleCreateOrUpdate) Client() IAMClient {
	return a.client
}

func (a RoleCreateOrUpdate) Do() error {
	client := a.Client()
	_, err := client.CreateRole(context.Background(), &a.CreateRole)
	if err == nil {
		fmt.Printf("Role %s created\n", *a.CreateRole.RoleName)
	}
	for _, cmd := range a.ManagedPolicies {
		_, err = client.AttachRolePolicy(context.Background(), &cmd)
		if err != nil {
			return err
		}
	}
	for _, cmd := range a.InlinePolicies {
		_, err = client.PutRolePolicy(context.Background(), &cmd)
		if err != nil {
			return err
		}
	}
	return err
}

func AttachRolePolicies(l Lambda) []iam.AttachRolePolicyInput {
	var inputs []iam.AttachRolePolicyInput
	inputs = append(inputs, iam.AttachRolePolicyInput{
		PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
		RoleName:  aws.String(l.ExecutionRole.RoleName),
	})
	for _, policy := range l.ExecutionRole.ManagedPolicies {
		inputs = append(inputs, l.ExecutionRole.AttachManagedPolicyCommand(policy))
	}
	return inputs
}

func PutRolePolicyCommand(l Lambda) []iam.PutRolePolicyInput {
	var inputs []iam.PutRolePolicyInput
	if l.ExecutionRole.InLinePolicy == "" {
		return inputs
	}
	cmd := iam.PutRolePolicyInput{
		PolicyName:     aws.String("glambda_inline_policy_" + UUID()),
		PolicyDocument: aws.String(l.ExecutionRole.InLinePolicy),
		RoleName:       aws.String(l.ExecutionRole.RoleName),
	}
	inputs = append(inputs, cmd)
	return inputs
}

func (l Lambda) PrepareRoleAction(client IAMClient) (RoleAction, error) {
	action := RoleCreateOrUpdate{
		client:          client,
		ManagedPolicies: []iam.AttachRolePolicyInput{},
		InlinePolicies:  []iam.PutRolePolicyInput{},
	}
	_, err := client.GetRole(context.Background(), &iam.GetRoleInput{
		RoleName: aws.String(l.ExecutionRole.RoleName),
	})
	if err != nil {
		var resourceNotFound *iTypes.NoSuchEntityException
		if errors.As(err, &resourceNotFound) {
			action = RoleCreateOrUpdate{
				client:     client,
				CreateRole: l.ExecutionRole.CreateRoleCommand(),
			}
		} else {
			return nil, err
		}
	}
	action.ManagedPolicies = AttachRolePolicies(l)
	action.InlinePolicies = PutRolePolicyCommand(l)
	return action, nil
}

func (l Lambda) PrepareLambdaAction(c LambdaClient) (LambdaAction, error) {
	exists, err := lambdaExists(c, l.Name)
	if err != nil {
		return nil, err
	}
	pkg, err := Package(l.HandlerPath)
	if err != nil {
		return nil, err
	}
	var action LambdaAction
	if exists {
		action = LambdaUpdateAction{
			Name: l.Name,
			Pkg:  pkg,
		}
	} else {
		action = LambdaCreateAction{
			Name: l.Name,
			Role: l.ExecutionRole.RoleName,
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

func CreateLambdaCommand(name, role string, pkg []byte) lambda.CreateFunctionInput {
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

func UpdateLambdaCommand(name string, pkg []byte) lambda.UpdateFunctionCodeInput {
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
