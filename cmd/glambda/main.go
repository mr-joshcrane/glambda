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
	err := glambda.Deploy(name, source)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Deployed successfully!")
}
