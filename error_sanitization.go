package main

import "regexp"

var (
	sensitiveQueryValue = regexp.MustCompile(`(?i)(xsec_token|keyword)=([^&\s]+)`)
	sensitiveJSONValue  = regexp.MustCompile(`(?i)("xsec_token"\s*:\s*")[^"]+`)
)

func safeErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := sensitiveQueryValue.ReplaceAllString(err.Error(), `$1=<REDACTED>`)
	return sensitiveJSONValue.ReplaceAllString(text, `$1<REDACTED>`)
}
