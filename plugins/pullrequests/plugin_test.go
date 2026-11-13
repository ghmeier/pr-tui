package pullrequests

import (
	"flag"
	"strings"
	"testing"

	"github.com/ghmeier/pr-tui/internal/plugin"
)

func TestPluginRegistersItselfUnderTheNameOfItsConfigurationTable(t *testing.T) {
	var names []string
	for _, registered := range plugin.Registered() {
		names = append(names, registered.Name())
	}

	if !strings.Contains(strings.Join(names, ","), pluginName) {
		t.Fatalf("registered plugins = %v, expected %s", names, pluginName)
	}
}

func TestPluginDeclaresTheRepoAndBotsFlags(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)

	(&pullRequestsPlugin{}).Flags(flags)

	if flags.Lookup("repo") == nil || flags.Lookup("bots") == nil {
		t.Fatalf("flags declared: repo = %v, bots = %v", flags.Lookup("repo") != nil, flags.Lookup("bots") != nil)
	}
}

func TestSetupRegistersTheBoardUnderTheNameOtherPluginsUse(t *testing.T) {
	provider := newFakeProvider()
	adder := &ticketPlugin{}

	_, err := newSetup(provider).alongside(adder).host(t)

	if err != nil {
		t.Fatalf("a plugin adding to the board %q could not find it: %v", BoardName, err)
	}
}

func TestSetupRejectsAnUnknownKeyOfItsTableNamingTheFileOnce(t *testing.T) {
	registerTestProvider(t, "fake", func(SectionDecoder) (Provider, error) { return newFakeProvider(), nil })
	pullRequests := &pullRequestsPlugin{}

	_, err := newHostFromText(t, "[pull_requests]\nprovider = \"fake\"\nclone_rot = \"~/code\"\n", pullRequests)

	if err == nil || !strings.Contains(err.Error(), "clone_rot") || strings.Count(err.Error(), "config.toml") != 1 {
		t.Fatalf("error = %v", err)
	}
}

func TestSetupFailsWhenTheProviderIsNotRegistered(t *testing.T) {
	_, err := newHostFromText(t, "[pull_requests]\nprovider = \"gitlab\"\n", &pullRequestsPlugin{})

	if err == nil || !strings.Contains(err.Error(), `"gitlab"`) {
		t.Fatalf("error = %v", err)
	}
}
