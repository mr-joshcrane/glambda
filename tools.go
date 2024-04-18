//go:build tools

package glambda

import (
	_ "github.com/aws/aws-lambda-go/lambda"
)

// This file stops mod tidy from cleaning up our "unused" imports
// We need the lambda package to be imported so we can work with user source files
