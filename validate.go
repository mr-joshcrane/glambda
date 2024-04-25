package glambda

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

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
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == "main" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func containsLambdaStartFunctionCall(node ast.Node) bool {
	var found bool
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			switch y := x.Fun.(type) {
			case *ast.SelectorExpr:
				if strings.HasPrefix(y.Sel.Name, "Start") {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}
