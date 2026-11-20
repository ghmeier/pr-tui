package workflows

import (
	"strings"
	"testing"
)

func TestNewProviderBuildsTheRegisteredProvider(t *testing.T) {
	expected := newFakeProvider()
	registerTestProvider(t, "fake", func() (Provider, error) { return expected, nil })

	provider, err := newProvider("fake")

	if err != nil || provider != Provider(expected) {
		t.Fatalf("provider = %v, error = %v", provider, err)
	}
}

func TestNewProviderRejectsAnUnknownNameNamingTheKeyAndTheSupportedProviders(t *testing.T) {
	registerTestProvider(t, "fake", func() (Provider, error) { return newFakeProvider(), nil })

	_, err := newProvider("gitlab")

	if err == nil || !strings.Contains(err.Error(), "[workflows] provider") || !strings.Contains(err.Error(), `"gitlab"`) || !strings.Contains(err.Error(), "fake") {
		t.Fatalf("error = %v", err)
	}
}

func TestRegisteringAProviderNameTwiceIsAProgrammingError(t *testing.T) {
	registerTestProvider(t, "fake", func() (Provider, error) { return newFakeProvider(), nil })
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()

	RegisterProvider("fake", func() (Provider, error) { return newFakeProvider(), nil })
}
