package glambda

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
)

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

type Lambda struct {
	Name           string
	HandlerPath    string
	ExecutionRole  ExecutionRole
	AWSAccountID   string
	CallingService string
	cfg            aws.Config
}

func customRetryer() aws.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = 10
		o.Retryables = append(o.Retryables, RetryableErrors{})
	})
}

type RetryableErrors struct{}

func (r RetryableErrors) IsErrorRetryable(err error) aws.Ternary {
	var opErr *smithy.OperationError
	if errors.As(err, &opErr) {
		var lambdaErr *types.InvalidParameterValueException
		if errors.As(err, &lambdaErr) {
			return aws.TrueTernary
		}
	}
	return aws.FalseTernary
}

func NewLambda(name, handlerPath string) (*Lambda, error) {
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRetryer(func() aws.Retryer {
			return customRetryer()
		}))
	if err != nil {
		return nil, err
	}

	accountID, err := AWSAccountID(sts.NewFromConfig(awsConfig))
	if err != nil {
		return nil, err
	}
	roleName := "glambda_exec_role_" + strings.ToLower(name)
	roleARN := "arn:aws:iam::" + accountID + ":role/" + roleName
	return &Lambda{
		Name:           name,
		HandlerPath:    handlerPath,
		CallingService: "events.amazonaws.com",
		ExecutionRole: ExecutionRole{
			RoleName:                 roleName,
			RoleARN:                  roleARN,
			AssumeRolePolicyDocument: DefaultAssumeRolePolicy,
			ManagedPolicies: []string{
				"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
			},
		},
		cfg:          awsConfig,
		AWSAccountID: accountID,
	}, nil
}

type ExecutionRole struct {
	RoleName                 string
	RoleARN                  string
	AssumeRolePolicyDocument string
	ManagedPolicies          []string
	InLinePolicy             string
}

func (e ExecutionRole) CreateRoleCommand() *iam.CreateRoleInput {
	return &iam.CreateRoleInput{
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

func (l Lambda) CreateLambdaResourcePolicy() lambda.AddPermissionInput {
	return lambda.AddPermissionInput{
		Action:        aws.String("lambda:InvokeFunction"),
		FunctionName:  aws.String(l.Name),
		StatementId:   aws.String("glambda_invoke_permission_" + UUID()),
		Principal:     aws.String(l.CallingService),
		SourceAccount: aws.String(l.AWSAccountID),
	}
}

type RoleOptions func(role *ExecutionRole)

func ExpandManagedPolicies(policyARNs []string) []string {
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

type LambdaClient interface {
	CreateFunction(ctx context.Context, params *lambda.CreateFunctionInput, optFns ...func(*lambda.Options)) (*lambda.CreateFunctionOutput, error)
	UpdateFunctionCode(ctx context.Context, params *lambda.UpdateFunctionCodeInput, optFns ...func(*lambda.Options)) (*lambda.UpdateFunctionCodeOutput, error)
	GetFunction(ctx context.Context, params *lambda.GetFunctionInput, optFns ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error)
	PublishVersion(ctx context.Context, params *lambda.PublishVersionInput, optFns ...func(*lambda.Options)) (*lambda.PublishVersionOutput, error)
	Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
	AddPermission(ctx context.Context, params *lambda.AddPermissionInput, optFns ...func(*lambda.Options)) (*lambda.AddPermissionOutput, error)
}

type IAMClient interface {
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	AttachRolePolicy(ctx context.Context, params *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
	PutRolePolicy(ctx context.Context, params *iam.PutRolePolicyInput, optFns ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
}

type STSClient interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

type Action interface {
	Do() error
}

type LambdaAction interface {
	Client() LambdaClient
	Action
}

type LambdaCreateAction struct {
	client                LambdaClient
	CreateLambdaCommand   *lambda.CreateFunctionInput
	ResourcePolicyCommand lambda.AddPermissionInput
	Name                  string
}

func NewLambdaCreateAction(client LambdaClient, l Lambda, pkg []byte) LambdaCreateAction {
	return LambdaCreateAction{
		client:                client,
		CreateLambdaCommand:   CreateLambdaCommand(l.Name, l.ExecutionRole.RoleARN, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
		Name:                  l.Name,
	}
}

func (a LambdaCreateAction) Client() LambdaClient {
	return a.client
}

func (a LambdaCreateAction) Do() error {
	client := a.Client()
	_, err := client.CreateFunction(context.Background(), a.CreateLambdaCommand)
	if err != nil {
		return err
	}
	_, err = client.AddPermission(context.Background(), &a.ResourcePolicyCommand)
	return err
}

type LambdaUpdateAction struct {
	client                LambdaClient
	UpdateLambdaCommand   *lambda.UpdateFunctionCodeInput
	ResourcePolicyCommand lambda.AddPermissionInput
	Name                  string
}

func NewLambdaUpdateAction(client LambdaClient, l Lambda, pkg []byte) LambdaUpdateAction {
	return LambdaUpdateAction{
		client:                client,
		UpdateLambdaCommand:   UpdateLambdaCommand(l.Name, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
		Name:                  l.Name,
	}
}

func (a LambdaUpdateAction) Client() LambdaClient {
	return a.client
}

func (a LambdaUpdateAction) Do() error {
	client := a.Client()
	_, err := client.UpdateFunctionCode(context.Background(), a.UpdateLambdaCommand)
	if err == nil {
		fmt.Printf("Lambda function %s updated\n", a.Name)
	}
	return err
}

func (l Lambda) Deploy() error {
	iamClient := iam.NewFromConfig(l.cfg)
	roleAction, err := PrepareRoleAction(l.ExecutionRole, iamClient)
	if err != nil {
		return err
	}
	err = roleAction.Do()
	if err != nil {
		return err
	}
	lambdaClient := lambda.NewFromConfig(l.cfg)
	action, err := PrepareLambdaAction(l, lambdaClient)
	if err != nil {
		return err
	}
	return action.Do()
}

func (l Lambda) Test() error {
	lambdaClient := lambda.NewFromConfig(l.cfg)
	version, err := WaitForConsistency(lambdaClient, l.Name)
	if err != nil {
		return err
	}
	_, err = lambdaClient.Invoke(context.Background(), &lambda.InvokeInput{
		FunctionName:   aws.String(l.Name),
		Qualifier:      aws.String(version),
		InvocationType: types.InvocationTypeDryRun,
	})
	return err
}

type RoleAction interface {
	Client() IAMClient
	Do() error
}

func NewRoleCreateOrUpdateAction(client IAMClient) RoleCreateOrUpdate {
	return RoleCreateOrUpdate{
		client:          client,
		CreateRole:      nil,
		ManagedPolicies: []iam.AttachRolePolicyInput{},
		InlinePolicies:  []iam.PutRolePolicyInput{},
	}
}

type RoleCreateOrUpdate struct {
	client          IAMClient
	CreateRole      *iam.CreateRoleInput
	ManagedPolicies []iam.AttachRolePolicyInput
	InlinePolicies  []iam.PutRolePolicyInput
}

func (a RoleCreateOrUpdate) Client() IAMClient {
	return a.client
}

func (a RoleCreateOrUpdate) Do() error {
	var err error
	client := a.Client()
	if a.CreateRole != nil {
		_, err := client.CreateRole(context.Background(), a.CreateRole)
		if err != nil {
			return err
		}
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

func PrepareRoleAction(role ExecutionRole, iamClient IAMClient) (RoleAction, error) {
	action := RoleCreateOrUpdate{
		client:         iamClient,
		InlinePolicies: []iam.PutRolePolicyInput{},
		ManagedPolicies: []iam.AttachRolePolicyInput{
			{
				PolicyArn: aws.String("arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"),
				RoleName:  aws.String(role.RoleName),
			},
		},
	}
	_, err := iamClient.GetRole(context.Background(), &iam.GetRoleInput{
		RoleName: aws.String(role.RoleName),
	})
	if err != nil {
		var resourceNotFound *iTypes.NoSuchEntityException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		action.CreateRole = role.CreateRoleCommand()
	}
	for _, policy := range role.ManagedPolicies {
		action.ManagedPolicies = append(action.ManagedPolicies, role.AttachManagedPolicyCommand(policy))
	}
	action.InlinePolicies = PutRolePolicyCommand(role)
	return action, nil
}

func PrepareLambdaAction(l Lambda, c LambdaClient) (LambdaAction, error) {
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
		action = NewLambdaUpdateAction(c, l, pkg)
	} else {
		action = NewLambdaCreateAction(c, l, pkg)
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

func CreateLambdaCommand(name, roleARN string, pkg []byte) *lambda.CreateFunctionInput {
	return &lambda.CreateFunctionInput{
		FunctionName: aws.String(name),
		Role:         aws.String(roleARN),
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

func UpdateLambdaCommand(name string, pkg []byte) *lambda.UpdateFunctionCodeInput {
	return &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(name),
		ZipFile:      pkg,
		Publish:      true,
	}
}

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

func Deploy(name, source string) error {
	l, err := NewLambda(name, source)
	if err != nil {
		return err
	}
	err = l.Deploy()
	if err != nil {
		return err
	}
	return l.Test()
}
