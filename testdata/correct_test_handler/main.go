package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.StartHandlerFunc(handler)
}

func handler(ctx context.Context, s any) (any, error) {
	fmt.Println("Hello, World!")
	return "Hello, World!", nil
}
