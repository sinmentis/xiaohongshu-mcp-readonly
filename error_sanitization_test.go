package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeErrorTextRedactsSensitiveQueryValues(t *testing.T) {
	err := errors.New(
		`navigate https://example.test/feed?xsec_token=secret-token&keyword=private-search`,
	)

	text := safeErrorText(err)

	assert.NotContains(t, text, "secret-token")
	assert.NotContains(t, text, "private-search")
	assert.Contains(t, text, "xsec_token=<REDACTED>")
	assert.Contains(t, text, "keyword=<REDACTED>")
}

func TestSafeErrorTextRedactsSensitiveJSONValues(t *testing.T) {
	err := errors.New(`request failed: {"xsec_token":"secret-token"}`)

	text := safeErrorText(err)

	assert.NotContains(t, text, "secret-token")
	assert.Contains(t, text, `"xsec_token":"<REDACTED>"`)
}
