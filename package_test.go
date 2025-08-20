//go:build full

package glambda_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/mr-joshcrane/glambda"
	mock "github.com/mr-joshcrane/glambda/testdata/mock_clients"
)

func TestPackage_PackagesLambdaFunction(t *testing.T) {
	t.Parallel()
	handler := "testdata/correct_test_handler/main.go"
	buf := new(bytes.Buffer)
	err := glambda.PackageTo(handler, buf)
	if err != nil {
		t.Error(err)
	}
	if len(buf.Bytes()) == 0 {
		t.Fatal("expected non-empty zip file")
	}
	checkZipFile(t, buf.Bytes())
}

func TestPrepareAction_CreateFunction(t *testing.T) {
	t.Parallel()

	client := mock.DummyLambdaClient{
		FuncExists: false,
		Err:        nil,
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}
	action, err := glambda.PrepareLambdaAction(l, client)
	if err != nil {YDo 
		t.Fatal(err)
	}
	_, ok := action.(glambda.LambdaCreateAction)
	if !ok {
		t.Errorf("expected CreateAction but did not get it")
	}

}

func TestPrepareAction_UpdateFunction(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: true,
		Err:        nil,
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}

	action, err := glambda.PrepareLambdaAction(l, client)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := action.(glambda.LambdaUpdateAction)
	if !ok {
		t.Errorf("expected UpdateAction but did not get it")
	}
}

func TestPrepareAction_ErrorCase(t *testing.T) {
	t.Parallel()
	client := mock.DummyLambdaClient{
		FuncExists: false,
		Err:        fmt.Errorf("some client error"),
	}
	handler := "testdata/correct_test_handler/main.go"
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}
	_, err := glambda.PrepareLambdaAction(l, client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
