package glambda

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
)

var principalRegex = regexp.MustCompile(`"Principal":\{(?:("AWS":\[(.*?)\])|("Service":"(.*?)"))\}`)
var arnConditionRegex = regexp.MustCompile(`"ArnLike":\{"AWS:SourceArn":"([^"]+)"\}`)
var accountConditionRegex = regexp.MustCompile(`"StringEquals":\{"AWS:SourceAccount":"([^"]+)"\}`)
var orgIdConditionRegex = regexp.MustCompile(`"StringEquals":\{"aws:PrincipalOrgID":"([^"]+)"\}`)

func removeQuotes(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	return strings.ReplaceAll(s, `'`, "")
}

func removeWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
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

func ParseManagedPolicy(policy string) []string {
	if policy == "" {
		return []string{}
	}
	policy = removeWhitespace(policy)
	policy = removeQuotes(policy)
	policies := strings.Split(policy, ",")
	var expandedPolicyArns []string
	for _, p := range policies {
		if strings.HasPrefix(p, "arn:") {
			expandedPolicyArns = append(expandedPolicyArns, p)
		} else {
			expandedPolicyArns = append(expandedPolicyArns, "arn:aws:iam::aws:policy/"+p)
		}
	}
	return expandedPolicyArns
}

func ParseInlinePolicy(policy string) (string, error) {
	if policy == "" {
		return "", fmt.Errorf("inlinePolicy is empty")
	}
	_, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("parsing failure for inlinePolicy: %w", err)
	}
	return removeWhitespace(policy), nil
}
