package pullrequests

import (
	"errors"
	"strings"
	"testing"
)

// registerTestProvider makes a provider selectable for one test.
func registerTestProvider(t *testing.T, name string, factory ProviderFactory) {
	t.Helper()
	RegisterProvider(name, factory)
	t.Cleanup(func() { delete(providerFactories, name) })
}

func TestNewProviderBuildsTheRegisteredProviderWithTheSectionDecoder(t *testing.T) {
	expected := newFakeProvider()
	var decoderGiven SectionDecoder
	registerTestProvider(t, "fake", func(decodeSection SectionDecoder) (Provider, error) {
		decoderGiven = decodeSection
		return expected, nil
	})
	decode := func(string, any) error { return nil }

	provider, err := NewProvider("fake", decode)

	if err != nil || provider != Provider(expected) || decoderGiven == nil {
		t.Fatalf("provider = %v, err = %v, decoder given = %v", provider, err, decoderGiven != nil)
	}
}

func TestNewProviderPassesTheFactoryFailureThrough(t *testing.T) {
	registerTestProvider(t, "fake", func(SectionDecoder) (Provider, error) { return nil, errorForTests })

	_, err := NewProvider("fake", nil)

	if !errors.Is(err, errorForTests) {
		t.Fatalf("error = %v", err)
	}
}

func TestNewProviderRejectsAnUnknownNameAndListsTheRegisteredOnes(t *testing.T) {
	registerTestProvider(t, "fake", func(SectionDecoder) (Provider, error) { return nil, nil })

	_, err := NewProvider("gitlab", nil)

	if err == nil || !strings.Contains(err.Error(), `"gitlab"`) || !strings.Contains(err.Error(), "fake") {
		t.Fatalf("error = %v, expected it to name gitlab and list fake", err)
	}
}

func TestRegisterProviderRefusesAnotherProviderUnderTheSameName(t *testing.T) {
	registerTestProvider(t, "fake", func(SectionDecoder) (Provider, error) { return nil, nil })

	defer func() {
		if recover() == nil {
			t.Fatal("registering a name twice should panic")
		}
	}()
	RegisterProvider("fake", func(SectionDecoder) (Provider, error) { return nil, nil })
}
