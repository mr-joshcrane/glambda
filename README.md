# Glambda Deployment Tool

Glambda is a simple tool for bundling and deploying AL2023 compatible Lambda functions written in Go. It provides an easy way to **create**, **update** and **delete** AWS Lambdas quickly from the command line using a compact set of commands. 

It's intended to maximise ease of use, at the expense of infinite customisability, and doesn't really play in the same space as SAM, CDK or Terraform.

## Why though?

AWS pivoted from a Go managed runtime to an OS only runtime. I'd argue it's not super intuitive to get started with these. Hence this libary!

You can learn more about it at https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html

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

Have AWS environment variables set up:

```bash
export AWS_ACCESS_KEY_ID=<your-access-key-id>
export AWS_SECRET_ACCESS_KEY=<your-secret-access-key>
export AWS_DEFAULT_REGION=<your-region>
```
---
## Create new lambdas
Run the following command to deploy a Lambda function with an associated
   execution role:

```bash
glambda deploy <lambdaName> <path/to/handler.go> 
```

Replace `<lambdaName>` with the desired name for your Lambda function and `<path/to/handler.go>` with the path to your Lambda function's handler file.

The source file should have a main function that calls lambda.Start(handler). 
See https://pkg.go.dev/github.com/aws/aws-lambda-go/lambda#Start for more details.

---
## Update existing lambdas

What's that? You've updated your code and need to deploy a new version of your Lambda function? No problem! Just run the same command as before, and Glambda will update the function code for you without recreating the lambda or the role.

In fact, assuming the path to your handler didnt change, we only need to run the same command!

```bash
glambda deploy <lambdaName> <path/to/handler.go>
```

---
## Execution Role and Lambda Resource Permissions

OK, that's nice, but sometimes your role actually has to DO things. Like access S3 buckets or DynamoDB tables. No problem! Glambda can attach managed policies, inline policies, and resource policies to your Lambda function's execution role. 

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
## Deleting lambdas and associated roles

Deleting your Lambda function and associated role is also easy, performed with
the following command:

```bash
glambda delete <lambdaName>
```
 
