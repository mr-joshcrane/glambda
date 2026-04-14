package glambda_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/mr-joshcrane/glambda"
	mock "github.com/mr-joshcrane/glambda/testdata/mock_clients"
)

func TestComputePlan_AllCreates(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		Functions:    []types.FunctionConfiguration{},
		FunctionTags: map[string]map[string]string{},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda: []glambda.LambdaDefinition{
			{Name: "funcA", Handler: "./a.go"},
			{Name: "funcB", Handler: "./b.go"},
		},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	creates, updates, deletes := plan.Summary()
	if creates != 2 {
		t.Errorf("expected 2 creates, got %d", creates)
	}
	if updates != 0 {
		t.Errorf("expected 0 updates, got %d", updates)
	}
	if deletes != 0 {
		t.Errorf("expected 0 deletes, got %d", deletes)
	}
}

func TestComputePlan_DetectsOrphanedLambdas(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		Functions: []types.FunctionConfiguration{
			{FunctionName: aws.String("oldFunc"), Runtime: types.RuntimeProvidedal2023},
		},
		FunctionTags: map[string]map[string]string{
			"oldFunc": {
				"ManagedBy":      "glambda",
				"GlambdaProject": "test-project",
			},
		},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda: []glambda.LambdaDefinition{
			{Name: "newFunc", Handler: "./new.go"},
		},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	creates, _, deletes := plan.Summary()
	if creates != 1 {
		t.Errorf("expected 1 create, got %d", creates)
	}
	if deletes != 1 {
		t.Errorf("expected 1 delete, got %d", deletes)
	}
	for _, item := range plan.Items {
		if item.Action == glambda.PlanDelete && item.Name != "oldFunc" {
			t.Errorf("expected delete of 'oldFunc', got %s", item.Name)
		}
	}
}

func TestComputePlan_DetectsConfigChange(t *testing.T) {
	t.Parallel()
	def := glambda.LambdaDefinition{Name: "myFunc", Handler: "./main.go", Timeout: 30}
	staleHash := "not-the-current-hash"

	client := mock.DummyLambdaClient{
		Functions: []types.FunctionConfiguration{
			{FunctionName: aws.String("myFunc"), Runtime: types.RuntimeProvidedal2023},
		},
		FunctionTags: map[string]map[string]string{
			"myFunc": {
				"ManagedBy":         "glambda",
				"GlambdaProject":    "test-project",
				"GlambdaConfigHash": staleHash,
			},
		},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda:  []glambda.LambdaDefinition{def},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	_, updates, _ := plan.Summary()
	if updates != 1 {
		t.Errorf("expected 1 update, got %d", updates)
	}
}

func TestComputePlan_AlwaysGeneratesUpdateForExistingLambdas(t *testing.T) {
	t.Parallel()
	def := glambda.LambdaDefinition{Name: "myFunc", Handler: "./main.go", Timeout: 30}
	currentHash := glambda.ConfigHash(def)

	client := mock.DummyLambdaClient{
		Functions: []types.FunctionConfiguration{
			{FunctionName: aws.String("myFunc"), Runtime: types.RuntimeProvidedal2023},
		},
		FunctionTags: map[string]map[string]string{
			"myFunc": {
				"ManagedBy":         "glambda",
				"GlambdaProject":    "test-project",
				"GlambdaConfigHash": currentHash,
			},
		},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda:  []glambda.LambdaDefinition{def},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasChanges() {
		t.Error("expected plan to always generate UPDATE for existing lambdas")
	}
	creates, updates, deletes := plan.Summary()
	if creates != 0 || updates != 1 || deletes != 0 {
		t.Errorf("expected 0 creates, 1 update, 0 deletes — got %d/%d/%d", creates, updates, deletes)
	}
}

func TestComputePlan_IgnoresOtherProjects(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		Functions: []types.FunctionConfiguration{
			{FunctionName: aws.String("otherProjectFunc"), Runtime: types.RuntimeProvidedal2023},
		},
		FunctionTags: map[string]map[string]string{
			"otherProjectFunc": {
				"ManagedBy":      "glambda",
				"GlambdaProject": "different-project",
			},
		},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda: []glambda.LambdaDefinition{
			{Name: "myFunc", Handler: "./main.go"},
		},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	creates, _, deletes := plan.Summary()
	if creates != 1 {
		t.Errorf("expected 1 create, got %d", creates)
	}
	if deletes != 0 {
		t.Errorf("expected 0 deletes (other project's lambda should be ignored), got %d", deletes)
	}
}

func TestComputePlan_IgnoresNonGlambdaFunctions(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		Functions: []types.FunctionConfiguration{
			{FunctionName: aws.String("terraformFunc"), Runtime: types.RuntimePython312},
		},
		FunctionTags: map[string]map[string]string{
			"terraformFunc": {
				"ManagedBy": "terraform",
			},
		},
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda: []glambda.LambdaDefinition{
			{Name: "myFunc", Handler: "./main.go"},
		},
	}
	plan, err := glambda.ComputePlan(cfg, client)
	if err != nil {
		t.Fatal(err)
	}
	_, _, deletes := plan.Summary()
	if deletes != 0 {
		t.Errorf("expected 0 deletes (non-glambda function should be ignored), got %d", deletes)
	}
}

func TestComputePlan_ErrorOnListFailure(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		Err: errForTest("API error"),
	}
	cfg := glambda.ProjectConfig{
		Project: glambda.ProjectMeta{Name: "test-project"},
		Lambda: []glambda.LambdaDefinition{
			{Name: "myFunc", Handler: "./main.go"},
		},
	}
	_, err := glambda.ComputePlan(cfg, client)
	if err == nil {
		t.Error("expected error on API failure")
	}
}

func TestPlan_HasChanges(t *testing.T) {
	t.Parallel()
	empty := glambda.Plan{ProjectName: "test"}
	if empty.HasChanges() {
		t.Error("empty plan should not have changes")
	}
	withItems := glambda.Plan{
		ProjectName: "test",
		Items:       []glambda.PlanItem{{Action: glambda.PlanCreate, Name: "fn"}},
	}
	if !withItems.HasChanges() {
		t.Error("plan with items should have changes")
	}
}

type testError string

func errForTest(msg string) error { return testError(msg) }
func (e testError) Error() string { return string(e) }
