package agentsessions

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

func TestSettingsDefaultToTheClaudeProviderAndTheHaikuRelayWhenTheTableIsMissing(t *testing.T) {
	loaded, err := loadSettings(configtest.Load(t, "").Section)

	if err != nil || loaded.providerName != "claude" || loaded.relayModel != "haiku" {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsReadTheProviderAndTheRelayModel(t *testing.T) {
	loaded, err := loadSettings(configtest.Load(t, "[agent_sessions]\nprovider = \"other\"\nrelay_model = \"sonnet\"\n").Section)

	if err != nil || loaded.providerName != "other" || loaded.relayModel != "sonnet" {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsRejectAnEmptyRelayModelNamingTheTableAndKey(t *testing.T) {
	_, err := loadSettings(configtest.Load(t, "[agent_sessions]\nrelay_model = \"\"\n").Section)

	if err == nil || !strings.Contains(err.Error(), "[agent_sessions] relay_model") {
		t.Fatalf("error = %v", err)
	}
}

func TestSettingsRejectAnUnknownKey(t *testing.T) {
	_, err := loadSettings(configtest.Load(t, "[agent_sessions]\nmodel = \"haiku\"\n").Section)

	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("error = %v", err)
	}
}
