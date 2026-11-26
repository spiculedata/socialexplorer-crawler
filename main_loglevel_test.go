package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultLogLevelIsInfo guards the invariant that the crawler defaults to
// info-level logging: debug output must not be produced in normal operation.
func TestDefaultLogLevelIsInfo(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("log-level")
	if flag == nil {
		t.Fatal("--log-level flag is not registered")
	}
	assert.Equal(t, "info", flag.DefValue,
		"the default --log-level must be info, not debug")
}
