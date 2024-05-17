package glambda

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
)

var principalRegex = regexp.MustCompile(`"Principal":\{(?:("AWS":\[(.*?)\])|("Service":"(.*?)"))\}`)
var arnConditionRegex = regexp.MustCompile(`"ArnLike":\{"AWS:SourceArn":"([^"]+)"\}`)
var accountConditionRegex = regexp.MustCompile(`"StringEquals":\{"AWS:SourceAccount":"([^"]+)"\}`)
var orgIdConditionRegex = regexp.MustCompile(`"StringEquals":\{"aws:PrincipalOrgID":"([^"]+)"\}`)

func removeWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ReplaceAll(s, "\r", "")
}

func ParseResourcePolicy(policy string) (ResourcePolicy, error) {
	var resourcePolicy ResourcePolicy
	policy = removeWhitespace(policy)

	// Match Principal
	principalMatch := principalRegex.FindStringSubmatch(policy)
	if len(principalMatch) > 0 {
		if principalMatch[2] != "" {
			resourcePolicy.Principal = fmt.Sprintf("{AWS:[%s]}", principalMatch[2])
		} else if principalMatch[4] != "" {
			resourcePolicy.Principal = fmt.Sprintf("{Service:%s}", principalMatch[4])
		}
	} else {
		return resourcePolicy, fmt.Errorf("principal not found in resource policy")
	}

	// Match ArnLike Condition
	arnConditionMatch := arnConditionRegex.FindStringSubmatch(policy)
	if len(arnConditionMatch) > 1 {
		resourcePolicy.SourceArnCondition = aws.String(arnConditionMatch[1])
	}

	// Match SourceAccount Condition
	accountConditionMatch := accountConditionRegex.FindStringSubmatch(policy)
	if len(accountConditionMatch) > 1 {
		resourcePolicy.SourceAccountCondition = aws.String(accountConditionMatch[1])
	}

	// Match PrincipalOrgID Condition
	orgIdConditionMatch := orgIdConditionRegex.FindStringSubmatch(policy)
	if len(orgIdConditionMatch) > 1 {
		resourcePolicy.PrincipalOrgIdCondition = aws.String(orgIdConditionMatch[1])
	}

	return resourcePolicy, nil
}
