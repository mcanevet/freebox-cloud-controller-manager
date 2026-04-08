package freebox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("FREEBOX_ENDPOINT", "https://mafreebox.freebox.fr")
	t.Setenv("FREEBOX_VERSION", "v8")
	t.Setenv("FREEBOX_APP_ID", "fr.freebox.ccm")
	t.Setenv("FREEBOX_TOKEN", "test-token-123")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "https://mafreebox.freebox.fr", cfg.Endpoint)
	assert.Equal(t, "v8", cfg.APIVersion)
	assert.Equal(t, "fr.freebox.ccm", cfg.AppID)
	assert.Equal(t, "test-token-123", cfg.AppToken)
}

func TestConfigFromEnv_MissingEndpoint(t *testing.T) {
	t.Setenv("FREEBOX_ENDPOINT", "")
	t.Setenv("FREEBOX_VERSION", "v8")
	t.Setenv("FREEBOX_APP_ID", "fr.freebox.ccm")
	t.Setenv("FREEBOX_TOKEN", "test-token")

	_, err := ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FREEBOX_ENDPOINT")
}

func TestConfigFromEnv_MissingVersion(t *testing.T) {
	t.Setenv("FREEBOX_ENDPOINT", "https://mafreebox.freebox.fr")
	t.Setenv("FREEBOX_VERSION", "")
	t.Setenv("FREEBOX_APP_ID", "fr.freebox.ccm")
	t.Setenv("FREEBOX_TOKEN", "test-token")

	_, err := ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FREEBOX_VERSION")
}

func TestConfigFromEnv_MissingAppID(t *testing.T) {
	t.Setenv("FREEBOX_ENDPOINT", "https://mafreebox.freebox.fr")
	t.Setenv("FREEBOX_VERSION", "v8")
	t.Setenv("FREEBOX_APP_ID", "")
	t.Setenv("FREEBOX_TOKEN", "test-token")

	_, err := ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FREEBOX_APP_ID")
}

func TestConfigFromEnv_MissingToken(t *testing.T) {
	t.Setenv("FREEBOX_ENDPOINT", "https://mafreebox.freebox.fr")
	t.Setenv("FREEBOX_VERSION", "v8")
	t.Setenv("FREEBOX_APP_ID", "fr.freebox.ccm")
	t.Setenv("FREEBOX_TOKEN", "")

	_, err := ConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FREEBOX_TOKEN")
}
