package main

import (
	"os"

	"github.com/mr-joshcrane/glambda/command"
)

func main() {
	err := command.Main(os.Args[1:])
	if err != nil {
		os.Exit(1)
	}
}
