package tickets

import (
	"cmp"
	"errors"
	"strings"
)

const defaultURLPrefix = "https://tickets.example.com/browse/"

// settings is the [tickets] table of the configuration file.
type settings struct {
	// URLPrefix is put in front of a ticket's id to make the address that opens it.
	URLPrefix string `toml:"url_prefix"`
}

// loadSettings reads the [tickets] table. The decoder is Host.Section, which rejects keys that
// settings does not declare.
func loadSettings(decodeSection func(name string, target any) error) (settings, error) {
	var section settings
	if err := decodeSection("tickets", &section); err != nil {
		return settings{}, err
	}
	section.URLPrefix = cmp.Or(section.URLPrefix, defaultURLPrefix)
	if !strings.HasPrefix(section.URLPrefix, "https://") && !strings.HasPrefix(section.URLPrefix, "http://") {
		return settings{}, errors.New(`[tickets] url_prefix must start with "https://" or "http://"`)
	}
	return section, nil
}
