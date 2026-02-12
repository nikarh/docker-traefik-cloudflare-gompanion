package main

import (
	"os"
	"path/filepath"
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

func TestGetSecretByEnvFromDefaultRunSecrets(t *testing.T) {
	const secretName = "CF_TOKEN"
	tempDir := t.TempDir()
	oldDirs := defaultSecretDirs
	defaultSecretDirs = []string{tempDir}
	t.Cleanup(func() {
		defaultSecretDirs = oldDirs
	})

	secretPath := filepath.Join(tempDir, secretName)
	err := os.WriteFile(secretPath, []byte("from-default-secret-file\n"), 0o600)
	require.NoError(t, err)

	t.Setenv(secretName, "")
	t.Setenv(secretName+"_FILE", "")

	value := getSecretByEnv(secretName)
	require.Equal(t, "from-default-secret-file", value)
}

func TestGetSecretByEnvFromFileEnvWithName(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gompanion-secret-*")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tempFile.Name()) }()

	_, err = tempFile.WriteString("from-file-env\n")
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	t.Setenv("CF_EMAIL_FILE", tempFile.Name())
	t.Setenv("CF_EMAIL", "")

	value := getSecretByEnv("CF_EMAIL")
	require.Equal(t, "from-file-env", value)
}
