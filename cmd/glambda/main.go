package main

import (
	"fmt"
	"os"

	"github.com/mr-joshcrane/glambda"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: glambda <lambdaName> <path/to/handler.go>")
		os.Exit(1)
	}

	name := os.Args[1]
	source := os.Args[2]
	inlinePolicy := ""
	executionRole := glambda.WithExecutionRole(
		"glambda_exec_role_"+name,
		glambda.WithInlinePolicy(inlinePolicy),
	)
	resourcePolicy := glambda.WithResourcePolicy("events.amazonaws.com")
	l := glambda.NewLambda(name, source, executionRole, resourcePolicy)
	err := l.Deploy()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Deployed successfully!")
}
