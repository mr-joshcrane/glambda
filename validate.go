package glambda

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// Validate takes a path to a Go source file.
// A valid Go source file for the purposes of AWS Lambda will...
//
// 1. Contain a main function.
//
// 2. Call one of the lambda Start... functions as seen here
// https://pkg.go.dev/github.com/aws/aws-lambda-go/lambda#Start
func Validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failure in reading %s: %w", path, err)
	}
	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, "main.go", string(data), parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failure in parsing %s: %w", path, err)
	}
	mainFound := containsMain(node)
	if !mainFound {
		return fmt.Errorf("main function not found in packaged function")
	}
	callsStart := containsLambdaStartFunctionCall(node)
	if !callsStart {
		return fmt.Errorf("main function does not call lambda.Start(handler)")
	}
	return nil
}

func containsMain(node ast.Node) bool {
	var found bool
	ast.Inspect(node, func(n ast.Node) bool {
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			if funcDecl.Name.Name == "main" {
				found = true
				// time to break out of the ast.Inspect travseral
				return false
			}
		}
		// continue traversing the ast looking for a main function
		return true
	})
	return found
}

func containsLambdaStartFunctionCall(node ast.Node) bool {
	var found bool
	ast.Inspect(node, func(n ast.Node) bool {
		if callExpr, ok := n.(*ast.CallExpr); ok {
			if selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				if strings.HasPrefix(selectorExpr.Sel.Name, "Start") {
					found = true
					// time to break out of the ast.Inspect travseral
					return false
				}
			}
		}
		// continue traversing the ast looking for a call to lambda.Start*
		return true
	})
	return found
}
