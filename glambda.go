package glambda

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type Lambda struct {
	Name           string
	HandlerPath    string
	ExecutionRole  ExecutionRole
	AWSAccountID   string
	ResourcePolicy ResourcePolicy
	cfg            aws.Config
}

type ResourcePolicy struct {
	Principal               string
	SourceAccountCondition  *string
	SourceArnCondition      *string
	PrincipalOrgIdCondition *string
}

type ExecutionRole struct {
	RoleName                 string
	RoleARN                  string
	AssumeRolePolicyDocument string
	ManagedPolicies          []string
	InLinePolicy             string
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
	if awsConfig.Region == "" {
		return nil, fmt.Errorf("unable to determine AWS region. Try setting the AWS_REGION environment variable")
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
		ResourcePolicy: ResourcePolicy{},
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

// Actions
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
	ResourcePolicyCommand *lambda.AddPermissionInput
}

func NewLambdaCreateAction(client LambdaClient, l Lambda, pkg []byte) LambdaCreateAction {
	return LambdaCreateAction{
		client:                client,
		CreateLambdaCommand:   CreateLambdaCommand(l.Name, l.ExecutionRole.RoleARN, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
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
	if a.ResourcePolicyCommand == nil {
		return nil
	}
	_, err = client.AddPermission(context.Background(), a.ResourcePolicyCommand)
	return err
}

type LambdaUpdateAction struct {
	client                LambdaClient
	UpdateLambdaCommand   *lambda.UpdateFunctionCodeInput
	ResourcePolicyCommand *lambda.AddPermissionInput
}

func NewLambdaUpdateAction(client LambdaClient, l Lambda, pkg []byte) LambdaUpdateAction {
	return LambdaUpdateAction{
		client:                client,
		UpdateLambdaCommand:   UpdateLambdaCommand(l.Name, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
	}
}

func (a LambdaUpdateAction) Client() LambdaClient {
	return a.client
}

func (a LambdaUpdateAction) Do() error {
	client := a.Client()
	_, err := client.UpdateFunctionCode(context.Background(), a.UpdateLambdaCommand)
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
		action.CreateRole = CreateRoleCommand(role.RoleName, role.AssumeRolePolicyDocument)
	}
	for _, policy := range role.ManagedPolicies {
		action.ManagedPolicies = append(action.ManagedPolicies, AttachManagedPolicyCommand(role.RoleName, policy))
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

type DeployOptions func(*Lambda) error

func WithManagedPolicies(policies string) DeployOptions {
	return func(l *Lambda) error {
		l.ExecutionRole.ManagedPolicies = ParseManagedPolicy(policies)
		return nil
	}
}

func WithInlinePolicy(policy string) DeployOptions {
	return func(l *Lambda) error {
		policy, err := ParseInlinePolicy(policy)
		if err != nil {
			return err
		}
		l.ExecutionRole.InLinePolicy = policy
		return nil
	}
}

func WithResourcePolicy(policy string) DeployOptions {
	return func(l *Lambda) error {
		if policy == "" {
			return nil
		}
		policy, err := ParseResourcePolicy(policy)
		if err != nil {
			return err
		}
		l.ResourcePolicy = policy
		return nil
	}
}

func WithAWSConfig(cfg aws.Config) DeployOptions {
	return func(l *Lambda) error {
		l.cfg = cfg
		return nil
	}
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

func Deploy(name, source string, opts ...DeployOptions) error {
	l, err := NewLambda(name, source)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		err := opt(l)
		if err != nil {
			return err
		}
	}
	err = l.Deploy()
	if err != nil {
		return err
	}
	return l.Test()
}

func Delete(name string) error {
	l, err := NewLambda(name, "")
	if err != nil {
		return err
	}
	lambdaClient := lambda.NewFromConfig(l.cfg)
	fnInfo, err := lambdaClient.GetFunction(context.Background(), &lambda.GetFunctionInput{
		FunctionName: aws.String(name),
	})
	if err != nil {
		return err
	}
	roleArn := *fnInfo.Configuration.Role
	_, err = lambdaClient.DeleteFunction(context.Background(), &lambda.DeleteFunctionInput{
		FunctionName: aws.String(name),
	})
	if err != nil {
		return err
	}
	iamClient := iam.NewFromConfig(l.cfg)
	roleName := strings.Split(roleArn, "/")[1]
	attachedPolicies, err := iamClient.ListAttachedRolePolicies(context.Background(), &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return err
	}
	for _, policy := range attachedPolicies.AttachedPolicies {
		_, err = iamClient.DetachRolePolicy(context.Background(), &iam.DetachRolePolicyInput{
			PolicyArn: policy.PolicyArn,
			RoleName:  aws.String(roleName),
		})
		if err != nil {
			return err
		}
	}
	_, err = iamClient.DeleteRole(context.Background(), &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	return err
}
