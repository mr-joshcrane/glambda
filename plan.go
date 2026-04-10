package glambda

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// PlanActionType describes what kind of change a PlanItem represents.
type PlanActionType string

const (
	PlanCreate PlanActionType = "CREATE"
	PlanUpdate PlanActionType = "UPDATE"
	PlanDelete PlanActionType = "DELETE"
)

// PlanItem represents a single intended change to an AWS Lambda function.
type PlanItem struct {
	Action PlanActionType
	Name   string
	Reason string
	// Definition is the desired config for CREATE/UPDATE actions. Nil for DELETE.
	Definition *LambdaDefinition
}

// Plan represents the full set of changes needed to reconcile AWS state
// with the desired project configuration.
type Plan struct {
	ProjectName string
	Items       []PlanItem
}

// HasChanges returns true if the plan contains any actions to perform.
func (p Plan) HasChanges() bool {
	return len(p.Items) > 0
}

// Summary returns counts of each action type in the plan.
func (p Plan) Summary() (creates, updates, deletes int) {
	for _, item := range p.Items {
		switch item.Action {
		case PlanCreate:
			creates++
		case PlanUpdate:
			updates++
		case PlanDelete:
			deletes++
		}
	}
	return
}

// ComputePlan compares the desired state from a ProjectConfig against the
// current state in AWS (queried via the LambdaClient) and produces a Plan
// describing what changes are needed.
//
// It identifies lambdas belonging to this project by the "GlambdaProject" tag,
// then determines which need to be created, updated, or deleted.
func ComputePlan(cfg ProjectConfig, client LambdaClient) (Plan, error) {
	plan := Plan{ProjectName: cfg.Project.Name}

	remote, err := listProjectFunctions(client, cfg.Project.Name)
	if err != nil {
		return plan, fmt.Errorf("listing project functions: %w", err)
	}

	// Build lookup of remote functions by name
	remoteByName := make(map[string]remoteLambda)
	for _, r := range remote {
		remoteByName[r.name] = r
	}

	// Check each desired lambda against remote state
	for _, def := range cfg.Lambda {
		r, exists := remoteByName[def.Name]
		if !exists {
			plan.Items = append(plan.Items, PlanItem{
				Action:     PlanCreate,
				Name:       def.Name,
				Reason:     "not found in AWS",
				Definition: &def,
			})
			continue
		}
		// Exists — check if config hash differs
		desiredHash := ConfigHash(def)
		if r.configHash != desiredHash {
			plan.Items = append(plan.Items, PlanItem{
				Action:     PlanUpdate,
				Name:       def.Name,
				Reason:     "config changed",
				Definition: &def,
			})
		}
		delete(remoteByName, def.Name)
	}

	// Anything left in remoteByName exists in AWS but not in config — delete
	for name := range remoteByName {
		plan.Items = append(plan.Items, PlanItem{
			Action: PlanDelete,
			Name:   name,
			Reason: "not in config",
		})
	}

	return plan, nil
}

type remoteLambda struct {
	name       string
	configHash string
}

func listProjectFunctions(client LambdaClient, projectName string) ([]remoteLambda, error) {
	var results []remoteLambda
	var marker *string

	for {
		output, err := client.ListFunctions(context.Background(), &lambda.ListFunctionsInput{
			Marker: marker,
		})
		if err != nil {
			return nil, err
		}

		for _, fn := range output.Functions {
			fnDetails, err := client.GetFunction(context.Background(), &lambda.GetFunctionInput{
				FunctionName: fn.FunctionName,
			})
			if err != nil {
				var resourceNotFound *types.ResourceNotFoundException
				if errors.As(err, &resourceNotFound) {
					continue
				}
				return nil, err
			}

			managedBy, ok := fnDetails.Tags["ManagedBy"]
			if !ok || managedBy != "glambda" {
				continue
			}

			project, ok := fnDetails.Tags["GlambdaProject"]
			if !ok || project != projectName {
				continue
			}

			results = append(results, remoteLambda{
				name:       aws.ToString(fn.FunctionName),
				configHash: fnDetails.Tags["GlambdaConfigHash"],
			})
		}

		if output.NextMarker == nil {
			break
		}
		marker = output.NextMarker
	}

	return results, nil
}

// ExecutePlan applies all the changes described in a Plan to AWS.
// Plan items are executed concurrently since each lambda is independent.
// On the first error, remaining work is cancelled and the error is returned.
func ExecutePlan(plan Plan, cfg ProjectConfig) error {
	if !plan.HasChanges() {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)

	for _, item := range plan.Items {
		wg.Add(1)
		go func(item PlanItem) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
			}

			var err error
			switch item.Action {
			case PlanCreate:
				err = deployWithProjectTags(plan.ProjectName, *item.Definition)
				if err != nil {
					err = fmt.Errorf("creating %s: %w", item.Name, err)
				}
			case PlanUpdate:
				err = deployWithProjectTags(plan.ProjectName, *item.Definition)
				if err != nil {
					err = fmt.Errorf("updating %s: %w", item.Name, err)
				}
			case PlanDelete:
				err = Delete(item.Name)
				if err != nil {
					err = fmt.Errorf("deleting %s: %w", item.Name, err)
				}
			}

			if err != nil {
				once.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(item)
	}

	wg.Wait()
	return firstErr
}

func deployWithProjectTags(projectName string, def LambdaDefinition) error {
	opts := def.ToDeployOptions()
	opts = append(opts, withProjectTags(projectName, def))
	return Deploy(def.Name, def.Handler, opts...)
}

// withProjectTags is a DeployOption that adds GlambdaProject and
// GlambdaConfigHash tags to the lambda function after deployment.
// It works by wrapping the deploy to tag the function post-creation.
func withProjectTags(projectName string, def LambdaDefinition) DeployOptions {
	return func(l *Lambda) error {
		l.Tags = map[string]string{
			"ManagedBy":         "glambda",
			"GlambdaProject":    projectName,
			"GlambdaConfigHash": ConfigHash(def),
		}
		return nil
	}
}

// PlanFromConfig is a convenience function that loads AWS config, creates
// clients, and computes a plan. It is the entry point used by the CLI.
func PlanFromConfig(projectCfg ProjectConfig) (Plan, error) {
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRetryer(customRetryer),
	)
	if err != nil {
		return Plan{}, err
	}
	if awsConfig.Region == "" {
		return Plan{}, fmt.Errorf("unable to determine AWS region. Try setting the AWS_DEFAULT_REGION environment variable")
	}
	_, err = AWSAccountID(sts.NewFromConfig(awsConfig))
	if err != nil {
		return Plan{}, err
	}
	lambdaClient := lambda.NewFromConfig(awsConfig)
	return ComputePlan(projectCfg, lambdaClient)
}

// ApplyFromConfig is a convenience function that loads the config file,
// computes a plan, and executes it. It is the entry point used by the CLI.
func ApplyFromConfig(projectCfg ProjectConfig) (Plan, error) {
	plan, err := PlanFromConfig(projectCfg)
	if err != nil {
		return plan, err
	}
	return plan, ExecutePlan(plan, projectCfg)
}
