package glambda_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mr-joshcrane/glambda"
	mock "github.com/mr-joshcrane/glambda/testdata/mock_clients"
)

func TestPackage_PackagesLambdaFunction(t *testing.T) {
	t.Parallel()
	handler := copyTestHandler(t)
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
	handler := copyTestHandler(t)
	l := glambda.Lambda{
		Name:          "test",
		HandlerPath:   handler,
		ExecutionRole: glambda.ExecutionRole{RoleName: "lambda-role"},
	}
	action, err := glambda.PrepareLambdaAction(l, client)
	if err != nil {
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
	handler := copyTestHandler(t)
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
	handler := copyTestHandler(t)
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

func checkZipFile(t *testing.T, zipContents []byte) {
	t.Helper()
	zipReader, err := zip.NewReader(bytes.NewReader(zipContents), int64(len(zipContents)))
	if err != nil {
		t.Errorf("failed to create zip reader, %v", err)
	}
	if len(zipReader.File) != 1 {
		t.Errorf("expected 1 file in zip, got %d", len(zipReader.File))
	}
	file := zipReader.File[0]
	if file.Name != "bootstrap" {
		t.Errorf("expected file name to be bootstrap, got %s", zipReader.File[0].Name)
	}
	if file.Mode() != 0o755 {
		t.Errorf("expected bootstrap file mode to be 0755, got %d", zipReader.File[0].Mode())
	}
	if file.UncompressedSize64 == 0 {
		t.Errorf("expected bootstrap file to have content, got 0")
	}

}
func copyTestHandler(t *testing.T) string {
	tempDir := t.TempDir()
	srcFile := "testdata/correct_test_handler/main.go"
	dstFile := filepath.Join(tempDir, "main.go")

	src, err := os.Open(srcFile)
	if err != nil {
		t.Fatalf("failed to open source file: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstFile)
	if err != nil {
		t.Fatalf("failed to create destination file: %v", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	return dstFile
}
