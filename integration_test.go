package glambda_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/mr-joshcrane/glambda"
)

var integration = flag.Bool("integration", false, "run integration tests against real AWS")

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if !*integration {
		t.Skip("skipping integration test; use -integration flag to run")
	}
}

func integrationRegion(t *testing.T) string {
	t.Helper()
	region := os.Getenv("GLAMBDA_TEST_REGION")
	if region == "" {
		t.Fatal("GLAMBDA_TEST_REGION must be set for integration tests (init() clobbers AWS_DEFAULT_REGION)")
	}
	return region
}

func newRealLambdaClient(t *testing.T, region string) *lambda.Client {
	t.Helper()
	awsCfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		t.Fatalf("loading AWS config: %v", err)
	}
	t.Logf("using AWS region: %s", awsCfg.Region)
	return lambda.NewFromConfig(awsCfg)
}

// writeHandlerFile creates a minimal valid Go Lambda handler in a temp directory
// and returns its path.
func writeHandlerFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := `package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.StartHandlerFunc(handler)
}

func handler(ctx context.Context, s any) (any, error) {
	fmt.Println("Hello, World!")
	return "Hello, World!", nil
}
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("writing handler file: %v", err)
	}
	return path
}

// waitForAWSConsistency gives AWS a moment to propagate tag changes.
func waitForAWSConsistency() {
	time.Sleep(5 * time.Second)
}

// getRemoteTags fetches the tags for a given Lambda function ARN or name.
func getRemoteTags(t *testing.T, client *lambda.Client, funcName string) map[string]string {
	t.Helper()
	out, err := client.GetFunction(context.Background(), &lambda.GetFunctionInput{
		FunctionName: aws.String(funcName),
	})
	if err != nil {
		t.Fatalf("getting function %s: %v", funcName, err)
	}
	return out.Tags
}

// functionExists returns true if the named Lambda function exists in AWS.
func functionExists(t *testing.T, client *lambda.Client, funcName string) bool {
	t.Helper()
	_, err := client.GetFunction(context.Background(), &lambda.GetFunctionInput{
		FunctionName: aws.String(funcName),
	})
	return err == nil
}

// cleanupProject force-deletes all lambdas belonging to the given project
// so tests leave no residue even on failure.
func cleanupProject(t *testing.T, projectName string, lambdaNames ...string) {
	t.Helper()
	for _, name := range lambdaNames {
		_ = glambda.Delete(name)
	}
}

func restoreRealGlobals(t *testing.T, region string) {
	t.Helper()
	savedUUID := glambda.UUID
	savedAccountID := glambda.AWSAccountID
	savedRetry := glambda.DefaultRetryWaitingPeriod
	savedRegion := os.Getenv("AWS_DEFAULT_REGION")

	glambda.UUID = glambda.GenerateUUID
	glambda.AWSAccountID = glambda.GetAWSAccountID
	glambda.DefaultRetryWaitingPeriod = func() { time.Sleep(3 * time.Second) }
	os.Setenv("AWS_DEFAULT_REGION", region)

	t.Cleanup(func() {
		glambda.UUID = savedUUID
		glambda.AWSAccountID = savedAccountID
		glambda.DefaultRetryWaitingPeriod = savedRetry
		os.Setenv("AWS_DEFAULT_REGION", savedRegion)
	})
}

func TestIntegration_FullLifecycle(t *testing.T) {
	skipUnlessIntegration(t)
	region := integrationRegion(t)
	restoreRealGlobals(t, region)

	projectName := "glambda-integ-test"
	handler := writeHandlerFile(t)
	lambdaClient := newRealLambdaClient(t, region)

	defer cleanupProject(t, projectName, "integ-alpha", "integ-beta", "integ-gamma", "integ-delta")

	t.Log("step 1: deploying 3 fresh lambdas (alpha, beta, gamma)")
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: projectName},
		Lambda: []glambda.LambdaDefinition{
			{Name: "integ-alpha", Handler: handler, Description: "Alpha function"},
			{Name: "integ-beta", Handler: handler, Description: "Beta function"},
			{Name: "integ-gamma", Handler: handler, Description: "Gamma function"},
		},
	}

	plan, err := glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 1: computing plan: %v", err)
	}

	creates, updates, deletes := plan.Summary()
	t.Logf("step 1: plan computed: creates=%d updates=%d deletes=%d", creates, updates, deletes)
	if creates != 3 || updates != 0 || deletes != 0 {
		t.Fatalf("step 1: expected 3/0/0, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}

	t.Log("step 1: executing plan...")
	err = glambda.ExecutePlan(plan, cfg)
	if err != nil {
		t.Fatalf("step 1: executing plan: %v", err)
	}
	t.Log("step 1: deploy complete, waiting for consistency...")

	waitForAWSConsistency()

	for _, name := range []string{"integ-alpha", "integ-beta", "integ-gamma"} {
		if !functionExists(t, lambdaClient, name) {
			t.Fatalf("step 1: expected %s to exist after deploy", name)
		}
		tags := getRemoteTags(t, lambdaClient, name)
		if tags["GlambdaProject"] != projectName {
			t.Fatalf("step 1: %s: expected GlambdaProject=%s, got %s", name, projectName, tags["GlambdaProject"])
		}
		if tags["ManagedBy"] != "glambda" {
			t.Fatalf("step 1: %s: expected ManagedBy=glambda, got %s", name, tags["ManagedBy"])
		}
		if tags["GlambdaConfigHash"] == "" {
			t.Fatalf("step 1: %s: expected non-empty GlambdaConfigHash", name)
		}
	}
	t.Log("step 1: all lambdas verified ✓")

	t.Log("step 2: re-running with identical config (expect no-op)")
	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 2: computing plan: %v", err)
	}

	if plan.HasChanges() {
		creates, updates, deletes = plan.Summary()
		t.Fatalf("step 2: expected no changes, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}
	t.Log("step 2: no-op confirmed ✓")

	t.Log("step 3: updating integ-beta timeout to 60s")
	cfg = glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: projectName},
		Lambda: []glambda.LambdaDefinition{
			{Name: "integ-alpha", Handler: handler, Description: "Alpha function"},
			{Name: "integ-beta", Handler: handler, Description: "Beta function", Timeout: 60},
			{Name: "integ-gamma", Handler: handler, Description: "Gamma function"},
		},
	}

	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 3: computing plan: %v", err)
	}

	creates, updates, deletes = plan.Summary()
	t.Logf("step 3: plan computed: creates=%d updates=%d deletes=%d", creates, updates, deletes)
	if creates != 0 || updates != 1 || deletes != 0 {
		t.Fatalf("step 3: expected 0/1/0, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}

	for _, item := range plan.Items {
		if item.Action == glambda.PlanUpdate && item.Name != "integ-beta" {
			t.Fatalf("step 3: expected update for integ-beta, got %s", item.Name)
		}
	}

	t.Log("step 3: executing plan...")
	err = glambda.ExecutePlan(plan, cfg)
	if err != nil {
		t.Fatalf("step 3: executing plan: %v", err)
	}
	t.Log("step 3: update complete ✓")

	waitForAWSConsistency()

	t.Log("step 4: removing gamma, adding delta")
	cfg = glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: projectName},
		Lambda: []glambda.LambdaDefinition{
			{Name: "integ-alpha", Handler: handler, Description: "Alpha function"},
			{Name: "integ-beta", Handler: handler, Description: "Beta function", Timeout: 60},
			{Name: "integ-delta", Handler: handler, Description: "Delta function"},
		},
	}

	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 4: computing plan: %v", err)
	}

	creates, updates, deletes = plan.Summary()
	t.Logf("step 4: plan computed: creates=%d updates=%d deletes=%d", creates, updates, deletes)
	if creates != 1 || updates != 0 || deletes != 1 {
		t.Fatalf("step 4: expected 1/0/1, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}

	for _, item := range plan.Items {
		switch item.Action {
		case glambda.PlanCreate:
			if item.Name != "integ-delta" {
				t.Fatalf("step 4: expected create for integ-delta, got %s", item.Name)
			}
		case glambda.PlanDelete:
			if item.Name != "integ-gamma" {
				t.Fatalf("step 4: expected delete for integ-gamma, got %s", item.Name)
			}
		}
	}

	t.Log("step 4: executing plan...")
	err = glambda.ExecutePlan(plan, cfg)
	if err != nil {
		t.Fatalf("step 4: executing plan: %v", err)
	}
	t.Log("step 4: create/delete complete, waiting for consistency...")

	waitForAWSConsistency()

	if functionExists(t, lambdaClient, "integ-gamma") {
		t.Fatal("step 4: expected integ-gamma to be deleted, but it still exists")
	}
	if !functionExists(t, lambdaClient, "integ-delta") {
		t.Fatal("step 4: expected integ-delta to exist after deploy")
	}
	t.Log("step 4: verified ✓")

	t.Log("step 5: verifying final state is consistent (expect no-op)")
	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 5: computing plan: %v", err)
	}

	if plan.HasChanges() {
		creates, updates, deletes = plan.Summary()
		t.Fatalf("step 5: expected no changes after final apply, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}
	t.Log("step 5: final consistency confirmed ✓")

	t.Log("step 6: empty config — expect plan to delete all 3 remaining lambdas")
	cfg = glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: projectName},
		Lambda:  []glambda.LambdaDefinition{},
	}

	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 6: computing plan: %v", err)
	}

	creates, updates, deletes = plan.Summary()
	t.Logf("step 6: plan computed: creates=%d updates=%d deletes=%d", creates, updates, deletes)
	if creates != 0 || updates != 0 || deletes != 3 {
		t.Fatalf("step 6: expected 0/0/3, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}

	t.Log("step 6: executing plan...")
	err = glambda.ExecutePlan(plan, cfg)
	if err != nil {
		t.Fatalf("step 6: executing plan: %v", err)
	}
	t.Log("step 6: delete-all complete, waiting for consistency...")

	waitForAWSConsistency()

	for _, name := range []string{"integ-alpha", "integ-beta", "integ-delta"} {
		if functionExists(t, lambdaClient, name) {
			t.Fatalf("step 6: expected %s to be deleted, but it still exists", name)
		}
	}
	t.Log("step 6: all lambdas deleted via empty config ✓")

	t.Log("step 7: verifying empty state is consistent (expect no-op)")
	plan, err = glambda.ComputePlan(cfg, lambdaClient)
	if err != nil {
		t.Fatalf("step 7: computing plan: %v", err)
	}

	if plan.HasChanges() {
		creates, updates, deletes = plan.Summary()
		t.Fatalf("step 7: expected no changes after empty-config apply, got creates=%d updates=%d deletes=%d", creates, updates, deletes)
	}
	t.Log("step 7: empty state consistency confirmed ✓")
}
