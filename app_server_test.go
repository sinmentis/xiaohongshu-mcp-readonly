package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateListenAddress(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:18060",
		"127.0.0.2:18060",
		"localhost:18060",
		"[::1]:18060",
	} {
		t.Run(address, func(t *testing.T) {
			require.NoError(t, validateListenAddress(address))
		})
	}
}

func TestValidateListenAddressRejectsNonLoopback(t *testing.T) {
	for _, address := range []string{
		":18060",
		"0.0.0.0:18060",
		"192.168.1.10:18060",
		"example.com:18060",
		"18060",
	} {
		t.Run(address, func(t *testing.T) {
			assert.Error(t, validateListenAddress(address))
		})
	}
}
