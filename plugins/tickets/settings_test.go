package tickets

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
)

func decoderFor(t *testing.T, text string) func(string, any) error {
	t.Helper()
	return configtest.Load(t, text).Section
}

func TestSettingsDefaultTheAddressPrefixWhenTheTableIsAbsent(t *testing.T) {
	loaded, err := loadSettings(decoderFor(t, ""))

	if err != nil || loaded.URLPrefix != defaultURLPrefix {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsReadTheConfiguredAddressPrefix(t *testing.T) {
	const prefix = "https://tracker.example.test/issues/"

	loaded, err := loadSettings(decoderFor(t, "[tickets]\nurl_prefix = \""+prefix+"\"\n"))

	if err != nil || loaded.URLPrefix != prefix {
		t.Fatalf("settings = %+v, error = %v", loaded, err)
	}
}

func TestSettingsRejectAnAddressPrefixWithoutAWebScheme(t *testing.T) {
	_, err := loadSettings(decoderFor(t, "[tickets]\nurl_prefix = \"tracker.example.test/\"\n"))

	if err == nil || !strings.Contains(err.Error(), "[tickets] url_prefix") {
		t.Fatalf("error = %v", err)
	}
}

func TestSettingsRejectAnUnknownKey(t *testing.T) {
	_, err := loadSettings(decoderFor(t, "[tickets]\nurl_prefx = \"https://x.example.test/\"\n"))

	if err == nil || !strings.Contains(err.Error(), "url_prefx") {
		t.Fatalf("error = %v", err)
	}
}
