package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTraefikV1HostRule(t *testing.T) {
	hosts := parseTraefikV1HostRule("Host:example.com,www.example.com")
	require.Equal(t, []string{"example.com", "www.example.com"}, hosts)
}

func TestParseTraefikV2Rule(t *testing.T) {
	hosts := parseTraefikV2Rule("Host(`a.example.com`) || Host(`b.example.com`)")
	require.Equal(t, []string{"a.example.com", "b.example.com"}, hosts)
}

func TestParseTraefikRouterRule(t *testing.T) {
	hosts := parseTraefikRouterRule("Host(`a.example.com`) && PathPrefix(`/foo`)")
	require.Equal(t, []string{"a.example.com"}, hosts)
}

func TestIsDomainExcluded(t *testing.T) {
	dom := DomainConfig{Name: "example.com", ExcludedSubDomains: []string{"internal", "dev"}}
	require.True(t, isDomainExcluded("api.internal.example.com", dom))
	require.False(t, isDomainExcluded("api.example.com", dom))
}

func TestParseBoolLikePython(t *testing.T) {
	require.True(t, parseBoolLikePython("TRUE", false))
	require.False(t, parseBoolLikePython("FALSE", true))
	require.True(t, parseBoolLikePython("not-a-bool", true))
}
