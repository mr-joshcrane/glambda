# Glambda Deployment Tool

Glambda is a simple tool for bundling and deploying AL2023 compatible Lambda functions written in Go. It provides an easy way to:

* create
* update
* delete

Lambda functions in the AWS ecosystem from the command line using a compact set of commands.

## Prerequisites

To use Glambda, you will need an AWS account, and an AWS Access Key ID and
Secret Access Key with the appropriate permissions to create and manage Lambda
functions, as well as IAM roles.

## Installation

To install Glambda, run:
```
go install github.com/mr-joshcrane/glambda/cmd/glambda@latest
```

## Usage

1. Have AWS environment variables set up:

```bash
export AWS_ACCESS_KEY_ID=<your-access-key-id>
export AWS_SECRET_ACCESS_KEY=<your-secret-access-key>
export AWS_DEFAULT_REGION=<your-region>
```

2. Run the following command to deploy a Lambda function with an associated
   execution role:

```bash
glambda deploy <lambdaName> <path/to/handler.go> 
```

Replace `<lambdaName>` with the desired name for your Lambda function and `<path/to/handler.go>` with the path to your Lambda function's handler file.


The source file should have a main function that calls lambda.Start(handler). 
See https://pkg.go.dev/github.com/aws/aws-lambda-go/lambda#Start for more details.

Optionally, you can specify the following flags:
```bash
## Attach a managed policy by name or ARN to the Lambda function's execution roles
managedPolicies=S3FullAccess,arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess

## Attach an inline policy (as a JSON literal) to the Lambda function's execution roles
inlinePolicies='{"Effect": "Deny", "Action": "s3:GetObject", "Resource": "*"}'

## Attach a resource policy (as a JSON literal) to the Lambda function
resourcePolicies='{
            "Sid": "YourLambdaResourcePolicy",
            "Effect": "Allow",
            "Principal": {
              "Service": "events.amazonaws.com"
            },
            "Action": "lambda:InvokeFunction",
            "Resource":  "arn:aws:lambda:us-east-2:123456789012:function:my-function",
            "Condition": {
              "StringEquals": {
                "AWS:SourceAccount": "123456789012"
              }
        }'
glambda deploy <lambdaName> <path/to/handler.go> \
    --managed-policies ${managedPolicies} \
    --inline-policy ${inlinePolicies} \
    --resource-policy ${resourcePolicies}
``` 

3. Delete your Lambda function and associated role is also easy, performed with
the following command:

```bash
glambda delete <lambdaName>
```
 
