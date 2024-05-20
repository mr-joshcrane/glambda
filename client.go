package glambda

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/smithy-go"
)

func customRetryer() aws.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = 20
		o.Retryables = append(o.Retryables, RetryableErrors{})
	})
}

type RetryableErrors struct{}

func (r RetryableErrors) IsErrorRetryable(err error) aws.Ternary {
	var opErr *smithy.OperationError
	if errors.As(err, &opErr) {
		var lambdaErr *types.InvalidParameterValueException
		if errors.As(err, &lambdaErr) {
			return aws.TrueTernary
		}
	}
	return aws.FalseTernary
}
