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

// Lambda is a struct that attempts to encapsulate the neccessary information
// required to deploy a lambda function to AWS. It doesn't map 1:1 with the
// AWS Lambda API, or any of the concrete AWS artifacts, and should be thought
// of as a higher level abstraction of convenience.
type Lambda struct {
	Name           string
	HandlerPath    string
	ExecutionRole  ExecutionRole
	AWSAccountID   string
	ResourcePolicy ResourcePolicy
	cfg            aws.Config
}

// ResourcePolicy is a struct that represents the policy that will be attached
// to the lambda function. Unlike the [Lambda] struct, this struct is more
// directly aligned to an AWS artifact. Namely the result of a call to the
// [AddPermission] API.
type ResourcePolicy struct {
	Principal               string
	SourceAccountCondition  *string
	SourceArnCondition      *string
	PrincipalOrgIdCondition *string
}

// ExecutionRole is a struct that attempts to encapsulate all the information
// required to create an AWS IAM Role that the lambda function will assume and
// operate on behalf of. It ties together several IAM concepts that would otherwise
// require separate API calls to create.
type ExecutionRole struct {
	RoleName                 string
	RoleARN                  string
	AssumeRolePolicyDocument string
	ManagedPolicies          []string
	InLinePolicy             string
}

// NewLambda is a constructor function that creates a new Lambda struct. It
// requires a friendly name for the lambda function to be created, and the path
// to the handler code that will be executed when the lambda function is invoked.
// It assumes the environment is configured with the necessary AWS credentials can
// be found in the enviroment. It also assumes that a default AWS region is set.
// Finally it assumes that the current AWS credentials can perform an
// sts:GetCallerIdentity identity call in order to determine the AWS account ID.
func NewLambda(name, handlerPath string) (*Lambda, error) {
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRetryer(customRetryer),
	)
	if err != nil {
		return nil, err
	}
	if awsConfig.Region == "" {
		return nil, fmt.Errorf("unable to determine AWS region. Try setting the AWS_DEFAULT_REGION environment variable")
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

// Actions are at a high level a way to organise a set of operations that need
// to be performed with the AWS SDK and in which order. Operations might depend
// on the result of a previous operation.
type Action interface {
	Do() error
}

// LambdaActions are any set of operations that requires the AWS Lambda service.
// It includes a configured client in order to perform these operations.
// They are a subset of the [Action] interface.
type LambdaAction interface {
	Client() LambdaClient
	Action
}

// LambdaCreateAction is [LambdaAction] that will create a new lambda function,
// and potentially attach a resource policy to it.
type LambdaCreateAction struct {
	client                LambdaClient
	CreateLambdaCommand   *lambda.CreateFunctionInput
	ResourcePolicyCommand *lambda.AddPermissionInput
}

// NewLambdaCreateAction is a constructor function that creates a new [LambdaCreateAction].
func NewLambdaCreateAction(client LambdaClient, l Lambda, pkg []byte) LambdaCreateAction {
	return LambdaCreateAction{
		client:                client,
		CreateLambdaCommand:   CreateLambdaCommand(l.Name, l.ExecutionRole.RoleARN, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
	}
}

// Client returns the required client type. In this case [LambdaClient].
func (a LambdaCreateAction) Client() LambdaClient {
	return a.client
}

// Do is the implementation of the [Action] interface. It will create the lambda
// function and attach the resource policy if it was provided, returning any error.
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

// LambdaUpdateAction is [LambdaAction] that will update an existing lambda function.
type LambdaUpdateAction struct {
	client                LambdaClient
	UpdateLambdaCommand   *lambda.UpdateFunctionCodeInput
	ResourcePolicyCommand *lambda.AddPermissionInput
}

// NewLambdaUpdateAction is a constructor function that creates a new [LambdaUpdateAction].
func NewLambdaUpdateAction(client LambdaClient, l Lambda, pkg []byte) LambdaUpdateAction {
	return LambdaUpdateAction{
		client:                client,
		UpdateLambdaCommand:   UpdateLambdaCommand(l.Name, pkg),
		ResourcePolicyCommand: l.CreateLambdaResourcePolicy(),
	}
}

// Client returns the required client type. In this case [LambdaClient].
func (a LambdaUpdateAction) Client() LambdaClient {
	return a.client
}

// Do is the implementation of the [Action] interface. It will update the lambda
// Updating a lambda function in this context will mean updating the packaged zip file
// that contains the lambda function code. It may also optionally require updating the
// resource policy attached to the lambda function, if one was provided.
func (a LambdaUpdateAction) Do() error {
	client := a.Client()
	_, err := client.UpdateFunctionCode(context.Background(), a.UpdateLambdaCommand)
	return err
}

// RoleAction is a high level interface that represents a set of operations that
// come from attempting to manage the AWS IAM Role that will be used as the
// Lambda's execution role.
type RoleAction interface {
	Client() IAMClient
	Do() error
}

// NewRoleCreateOrUpdateAction is a constructor function that creates a new [RoleCreateOrUpdate].
func NewRoleCreateOrUpdateAction(client IAMClient) RoleCreateOrUpdate {
	return RoleCreateOrUpdate{
		client:          client,
		CreateRole:      nil,
		ManagedPolicies: []iam.AttachRolePolicyInput{},
		InlinePolicies:  []iam.PutRolePolicyInput{},
	}
}

// RoleCreateOrUpdate is a struct that implements the [RoleAction] interface.
// It is a high level abstraction that encapsulates the operations required to
// either create or update an IAM Role. It includes the ability to attach managed
// policies and inline policies to the role.
//
// The reason create and update are combined into a single struct is because from
// the users perspective, the goal is the same. They want to ensure that the role
// exists and has the correct policies attached to it.
type RoleCreateOrUpdate struct {
	client          IAMClient
	CreateRole      *iam.CreateRoleInput
	ManagedPolicies []iam.AttachRolePolicyInput
	InlinePolicies  []iam.PutRolePolicyInput
}

// Client returns the required client type. In this case [IAMClient].
func (a RoleCreateOrUpdate) Client() IAMClient {
	return a.client
}

// Do is the implementation of the [Action] interface. It will create the role if
// it was determined that it didn't exist at Action construction time (see [PrepareRoleAction]).
// It will then execute the attach role policy and put role policy commands in that order
// as provided at Action construction time.
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

// PrepareRoleAction is a function that creates a new [RoleCreateOrUpdate] struct.
// It will add the AWS managed policy "AWSLambdaBasicExecutionRole" to the role by default
// as a lambda without this role makes very little sense. It will also add any managed
// policies and inline policies that were provided in the [ExecutionRole] struct.
//
// This function does make live API calls to AWS IAM to determine if the role already exists.
// If not, it will create a new [CreateRoleCommand] to be executed by the [RoleCreateOrUpdate].
// The [PutRolePolicyCommand] and [AttachManagedPolicyCommand] created here for deferred execution.
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

// PrepareLambdaAction is a function that creates a new [LambdaAction] struct.
// It will create the deployment package, and then determine if the lambda function
// needs to be created. It will branch out into either a [LambdaCreateAction] or
// a [LambdaUpdateAction] depending on the current state in AWS.
func PrepareLambdaAction(l Lambda, c LambdaClient) (LambdaAction, error) {
	pkg, err := Package(l.HandlerPath)
	if err != nil {
		return nil, err
	}
	exists, err := lambdaExists(c, l.Name)
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

// CreateLambdaCommand is a paperwork reducer that translates parameters into
// the smithy autogenerated AWS Lambda SDKv2 format of [lambda.CreateFunctionInput]
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

// UpdateLambdaCommand is a paperwork reducer that translates parameters into
// the smithy autogenerated AWS Lambda SDKv2 format of [lambda.UpdateFunctionCodeInput]
func UpdateLambdaCommand(name string, pkg []byte) *lambda.UpdateFunctionCodeInput {
	return &lambda.UpdateFunctionCodeInput{
		FunctionName: aws.String(name),
		ZipFile:      pkg,
		Publish:      true,
	}
}

// DeployOptions is any function that can be used to configure a [Lambda]
// struct before it is deployed. It is a functional option pattern.
type DeployOptions func(*Lambda) error

// WithManagedPolicies is a deploy option that allows the user to attach
// one or more managed policies to the [Lambda] struct. The managed policies
// are expected to be a comma separated string of ARNs. For parsing rules
// see [ParseManagedPolicy].
func WithManagedPolicies(policies string) DeployOptions {
	return func(l *Lambda) error {
		l.ExecutionRole.ManagedPolicies = ParseManagedPolicy(policies)
		return nil
	}
}

// WithInlinePolicy is a deploy option that allows the user to attach
// an inline policy to the [Lambda] struct. The inline policy is expected
// to be a JSON string. For parsing rules see [ParseInlinePolicy].
func WithInlinePolicy(policy string) DeployOptions {
	return func(l *Lambda) error {
		if policy == "" {
			return nil
		}
		policy, err := ParseInlinePolicy(policy)
		if err != nil {
			return err
		}
		l.ExecutionRole.InLinePolicy = policy
		return nil
	}
}

// WithResourcePolicy is a deploy option that allows the user to attach
// a resource policy to the [Lambda] struct. The resource policy is expected
// to be a JSON string. For parsing rules see [ParseResourcePolicy].
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

// WithAWSConfig is a deploy option that allows the user to provide a custom
// AWS Config to the [Lambda] struct. This is useful when you need more fine grained
// control over the AWS SDK configuration.
func WithAWSConfig(cfg aws.Config) DeployOptions {
	return func(l *Lambda) error {
		l.cfg = cfg
		return nil
	}
}

// Deploy is a method on the [Lambda] struct that will attempt to deploy the lambda
// function to AWS. It will attempt to prepare, then deploy the execution role, and
// if successful will repeat the process for the lambda function itself.
func (l Lambda) Deploy() error {
	l.cfg.Retryer = customRetryer
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

// Test is a method on the [Lambda] struct that will attempt to invoke the newly
// created lambda function in a dry run mode. This is useful for testing the lambda
// function after deployment. As per AWS documentation, the dry run mode should not
// execute the lambda function, but will rather 'validate parameter values and verify that the user or role has permission to invoke the function'.
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

// Deploy is a convenience function that will handle the paperwork that would
// otherwise fall to the user to manage. It will create a new [Lambda] struct
// and attempt to deploy it to AWS. It will also test the lambda function after
// deployment. It is a high level abstraction that should represent the majority
// of use cases for this library.
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

// Delete is a convenience function that will delete a lambda function and the
// associated IAM Role. Deletion is actually more complex than it might seem at
// first glance and requires a specific unwinding of various resources.
//
// It should be noted that it is A) a destructive operation and B) allows for the
// possibility of deleting resources that are not managed by this library. The usual
// care and due dillgence should be taken before deleting.
//
// It will also detach any managed policies that were attached
// to the role. It is a high level abstraction that should represent the majority
// of use cases for this library.
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
