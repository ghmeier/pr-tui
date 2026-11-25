package agentsessions

import (
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/config/configtest"
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/plugins/pullrequests/pullrequeststest"
)

func TestNewProviderBuildsTheRegisteredProviderWithTheSettings(t *testing.T) {
	expected := &fakeProvider{}
	registerFake(t, expected)

	provider, err := newProvider("fake", ProviderSettings{RelayModel: "sonnet"})

	if err != nil || provider != Provider(expected) || expected.settings.RelayModel != "sonnet" {
		t.Fatalf("provider = %v, error = %v, settings = %+v", provider, err, expected.settings)
	}
}

func TestNewProviderRejectsAnUnknownNameNamingTheKeyAndTheSupportedProviders(t *testing.T) {
	registerFake(t, &fakeProvider{})

	_, err := newProvider("other", ProviderSettings{})

	if err == nil || !strings.Contains(err.Error(), "[agent_sessions] provider") || !strings.Contains(err.Error(), `"other"`) || !strings.Contains(err.Error(), "fake") {
		t.Fatalf("error = %v", err)
	}
}

func TestRegisteringAProviderNameTwiceIsAProgrammingError(t *testing.T) {
	registerFake(t, &fakeProvider{})
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	RegisterProvider("fake", func(ProviderSettings) (Provider, error) { return &fakeProvider{}, nil })
}

func TestPluginPassesTheConfiguredRelayModelToTheProvider(t *testing.T) {
	provider := &fakeProvider{}
	registerFake(t, provider)
	fixture := newBoardFixture()
	pullRequestProvider := &pullrequeststest.Provider{Source: fixture.source}
	text := "plugins = [\"pull_requests\", \"agent_sessions\"]\n[pull_requests]\nprovider = \"" + pullrequeststest.Register(t, pullRequestProvider) +
		"\"\n[agent_sessions]\nprovider = \"fake\"\nrelay_model = \"sonnet\"\n"

	_, err := plugin.NewHost("config.toml", configtest.Load(t, text), plugin.Registered())

	if err != nil || provider.settings.RelayModel != "sonnet" {
		t.Fatalf("error = %v, settings = %+v", err, provider.settings)
	}
}

func TestPluginRejectsAnUnknownProviderNamingTheKey(t *testing.T) {
	pullRequestProvider := &pullrequeststest.Provider{Source: newBoardFixture().source}
	text := "plugins = [\"pull_requests\", \"agent_sessions\"]\n[pull_requests]\nprovider = \"" + pullrequeststest.Register(t, pullRequestProvider) +
		"\"\n[agent_sessions]\nprovider = \"other\"\n"

	_, err := plugin.NewHost("config.toml", configtest.Load(t, text), plugin.Registered())

	if err == nil || !strings.Contains(err.Error(), "[agent_sessions] provider") {
		t.Fatalf("error = %v", err)
	}
}

func TestPluginStartsWithoutAddingToTheBoardWhenPullRequestsAreNotEnabled(t *testing.T) {
	registerFake(t, &fakeProvider{})
	text := "plugins = [\"agent_sessions\"]\n[agent_sessions]\nprovider = \"fake\"\n"

	_, err := plugin.NewHost("config.toml", configtest.Load(t, text), plugin.Registered())

	if err != nil {
		t.Fatal(err)
	}
}
