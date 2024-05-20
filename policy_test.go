package glambda_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/google/go-cmp/cmp"
	"github.com/mr-joshcrane/glambda"
)

func TestParseResourcePolicy_ServicePoliciesWithConditions(t *testing.T) {
	testPolicy := `{
    "Version": "2012-10-17",
    "Id": "default",
    "Statement": [
        {
            "Sid": "lambda-allow-s3-my-function",
            "Effect": "Allow",
            "Principal": {
              "Service": "s3.amazonaws.com"
            },
            "Action": "lambda:InvokeFunction",
            "Resource":  "arn:aws:lambda:us-east-2:123456789012:function:my-function",
            "Condition": {
              "StringEquals": {
                "AWS:SourceAccount": "123456789012"
              },
              "ArnLike": {
                "AWS:SourceArn": "arn:aws:s3:::DOC-EXAMPLE-BUCKET"
              },
	            "StringEquals": {
                "aws:PrincipalOrgID": "o-a1b2c3d4e5f"
              }
            }
        }
     ]
}`
	l := &glambda.Lambda{}
	err := glambda.WithResourcePolicy(testPolicy)(l)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	want := glambda.ResourcePolicy{
		Principal:               `{Service:s3.amazonaws.com}`,
		SourceAccountCondition:  aws.String(`123456789012`),
		SourceArnCondition:      aws.String(`arn:aws:s3:::DOC-EXAMPLE-BUCKET`),
		PrincipalOrgIdCondition: aws.String(`o-a1b2c3d4e5f`),
	}
	if !cmp.Equal(l.ResourcePolicy, want) {
		t.Errorf(cmp.Diff(want, l.ResourcePolicy))
	}
}

func TestParseResourcePolicy_AWSPoliciesWithConditions(t *testing.T) {
	testPolicy := `{
    "Version": "2012-10-17",
    "Id": "default",
    "Statement": [
        {
            "Sid": "lambda-allow-s3-my-function",
            "Effect": "Allow",
            "Principal": {
              "AWS": [ 
 							 "123456789012",
  						 "555555555555" 
  						]
            },
            "Action": "lambda:InvokeFunction",
            "Resource":  "arn:aws:lambda:us-east-2:123456789012:function:my-function",
  					"Condition": {            
							"ArnLike": {
                "AWS:SourceArn": "arn:aws:s3:::DOC-EXAMPLE-BUCKET"
              },
	            "StringEquals": {
                "aws:PrincipalOrgID": "o-a1b2c3d4e5f"
              }
            }
        }
     ]
}`
	l := &glambda.Lambda{}
	err := glambda.WithResourcePolicy(testPolicy)(l)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	want := glambda.ResourcePolicy{
		Principal:               `{AWS:["123456789012","555555555555"]}`,
		SourceArnCondition:      aws.String(`arn:aws:s3:::DOC-EXAMPLE-BUCKET`),
		PrincipalOrgIdCondition: aws.String(`o-a1b2c3d4e5f`),
	}
	if !cmp.Equal(l.ResourcePolicy, want) {
		t.Errorf(cmp.Diff(want, l.ResourcePolicy))
	}
}

func TestParseResourcepolicy_MissingPrincipalTriggersAnError(t *testing.T) {
	t.Parallel()
	testPolicy := `{
    "Version": "2012-10-17",
    "Id": "default",
    "Statement": [
        {
            "Sid": "lambda-allow-s3-my-function",
            "Effect": "Allow",
            "Action": "lambda:InvokeFunction",
            "Resource":  "arn:aws:lambda:us-east-2:123456789012:function:my-function",
        }
     ]
}`
	l := &glambda.Lambda{}
	err := glambda.WithResourcePolicy(testPolicy)(l)
	if err == nil {
		t.Errorf("Expected error but got nil")
	}
}
