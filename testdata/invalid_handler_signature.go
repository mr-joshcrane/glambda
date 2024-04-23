package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

// This handler is invalid because it does not return anything
// Three arguments and no return is not a valid handler
func handler(ctx context.Context, event any, moreArgs string) {
	fmt.Println("Hello, World!")
}
