// Package search finds photos from a plain-language request: a model turns
// the request into filters checked against the team's own vocabulary, the
// database applies them, and text vectors rank what is left by meaning.
// The model never sees or invents photos; every result is a stored row.
package search

import (
	"fmt"
	"sort"
	"strings"

	"pidex/internal/teamconfig"
)

// DocInput is what a photo is searched by.
type DocInput struct {
	Description    string
	TagKeys        []string
	DocumentFields map[string]string
}

// Document is the text a photo is indexed by, in the team's language. It
// leaves out identity (names, record numbers): the text is sent to an
// embedding model, and subjects are found through their own table instead.
// Document fields are left out for high-privacy teams for the same reason.
func Document(cfg *teamconfig.Config, in DocInput) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(in.Description))
	var tags []string
	for _, k := range in.TagKeys {
		for _, t := range cfg.Tags {
			if t.Key != k {
				continue
			}
			if t.Group != "" {
				tags = append(tags, fmt.Sprintf("%s (%s)", t.Name, t.Group))
			} else {
				tags = append(tags, t.Name)
			}
		}
	}
	if len(tags) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(tags, ", "))
	}
	if cfg.Privacy != teamconfig.PrivacyHigh && len(in.DocumentFields) > 0 {
		keys := make([]string, 0, len(in.DocumentFields))
		for k := range in.DocumentFields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "\n%s: %s", k, in.DocumentFields[k])
		}
	}
	return strings.TrimSpace(b.String())
}
