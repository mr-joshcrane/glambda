package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, s string) (string, error) {
	fmt.Println("Hello, World!")
	return "Hello, World!", nil
}
