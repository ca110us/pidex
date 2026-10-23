package teamconfig

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed templates/*.yaml
var templateFS embed.FS

type Template struct {
	Key    string
	Config *Config
}

// Templates returns every built-in template, sorted by key.
func Templates() ([]Template, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	var out []Template
	for _, e := range entries {
		key := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		cfg, err := LoadTemplate(key)
		if err != nil {
			return nil, err
		}
		out = append(out, Template{Key: key, Config: cfg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// LoadTemplate returns a fresh copy of one built-in template.
func LoadTemplate(key string) (*Config, error) {
	if !keyPattern.MatchString(key) {
		return nil, fmt.Errorf("unknown template %q", key)
	}
	raw, err := templateFS.ReadFile("templates/" + key + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("unknown template %q", key)
	}
	return ParseYAML(raw)
}

// ParseYAML accepts the same structure as Parse, written as YAML.
func ParseYAML(data []byte) (*Config, error) {
	j, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("decode team config yaml: %w", err)
	}
	return Parse(j)
}
