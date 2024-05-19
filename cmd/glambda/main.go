package main

import (
	"os"

	"github.com/mr-joshcrane/glambda"
)

func main() {
	err := glambda.Main(os.Args[1:])
	if err != nil {
		os.Exit(1)
	}
}
