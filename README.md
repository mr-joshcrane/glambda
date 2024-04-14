# GLambda Deployment Tool

GLambda is a simple deployment tool for AWS Lambda functions written in Go. It provides a way to create and update Lambda functions in the AWS ecosystem from the command line using a compact set of commands.

## Prerequisites

To use GLambda, you will need an AWS account with appropriate permissions and an AWS CLI profile configured on your machine.

## Installation

To install GLambda, run:
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

You'll need IAM permissions, and lambda permissions.
By default a single shared role is created for all lambdas. This will be
extended in the future.

2. Give the function you wish to deploy a name and a source file to package up
   and deploy.

```bash
glambda <lambdaName> <path/to/handler.go>
```

Replace `<lambdaName>` with the desired name for your Lambda function and `<path/to/handler.go>` with the path to your Lambda function's handler file.


The source file should have a main function that calls lambda.Start(handler). 
See https://pkg.go.dev/github.com/aws/aws-lambda-go/lambda#Start for more details.

## Testing

GLambda comes with a set of unit tests to ensure its functionality. To run the tests, you can use the following command:
```
go test -v ./...
```

## Contributing

Contributions to GLambda are welcome! If you encounter any bugs or have ideas for new features, feel free to submit an issue or create a pull request.

## License

GLambda is open-source software licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.

---

Feel free to customize this README.md to include any additional information specific to your project.

