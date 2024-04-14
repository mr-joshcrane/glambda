package glambda

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
	dcl, mainFound := containsMain(node)
	if !mainFound {
		return fmt.Errorf("main function not found in packaged function")
	}
	callsStart := containsLambdaStartFunctionCall(*dcl)
	if !callsStart {
		return fmt.Errorf("main function does not call lambda.Start(handler)")
	}
	return nil
}

func containsMain(node ast.Node) (*ast.FuncDecl, bool) {
	var mainFunc *ast.FuncDecl
	var found bool
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Name.Name == "main" {
				mainFunc = x
				found = true
			}
		}
		return !found
	})
	return mainFunc, found
}

func containsLambdaStartFunctionCall(dcl ast.FuncDecl) bool {
	for _, stmt := range dcl.Body.List {
		switch x := stmt.(type) {
		case *ast.ExprStmt:
			switch y := x.X.(type) {
			case *ast.CallExpr:
				switch z := y.Fun.(type) {
				case *ast.SelectorExpr:
					if z.Sel.Name == "Start" {
						return true
					}
				}
			}
		}
	}
	return false
}
