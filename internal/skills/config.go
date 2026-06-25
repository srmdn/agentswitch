package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Roots   []RootPair
	Packs   map[string]PackLayout
	Presets map[string][]string
}

func DefaultConfigPath() string {
	if path := os.Getenv("AGENTSWITCH_CONFIG"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "agentswitch", "config.toml")
	}
	return filepath.Join(home, ".config", "agentswitch", "config.toml")
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return ParseConfig(string(data))
}

func InitConfig(path string, overwrite bool, minimal bool) error {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	template := DefaultConfigTOML()
	if minimal {
		template = MinimalConfigTOML()
	}
	return os.WriteFile(path, []byte(template), 0o644)
}

func DefaultConfigTOML() string {
	home := "$HOME"
	return strings.TrimSpace(fmt.Sprintf(`
# agentswitch config
#
# Skills are discovered from roots. Packs and presets are user-owned here, not
# hardcoded in the binary.

[[roots]]
name = "user"
active = "%s/.agents/skills"
disabled = "%s/.agents/skills.disabled"
switchable = true

[[roots]]
name = "codex"
active = "%s/.codex/skills"
disabled = "%s/.codex/skills.disabled"
switchable = true

[[roots]]
name = "admin"
active = "/etc/codex/skills"
switchable = false

[packs.go]
type = "directory"
active = "%s/.agents/skills/cc-skills-golang"
disabled = "%s/.agents/skills.disabled/cc-skills-golang"
skills_subdir = "skills"

[packs.translation]
type = "group"
active = "%s/.agents/skills"
disabled = "%s/.agents/skills.disabled/translation"
disabled_fallback = "%s/.agents/skills.disabled"
skills = [
  "id-locale-translation",
  "id-technical-docs-translation",
  "natural-id-translation",
]

[packs.wordpress]
type = "symlinked"
active = "%s/.codex/skills"
disabled = "%s/.codex/skills.disabled/wordpress-pack"
link_root = "%s/.agents/skills"
skills = [
  "blueprint",
  "wordpress-router",
  "wp-abilities-api",
  "wp-abilities-audit",
  "wp-abilities-verify",
  "wp-block-development",
  "wp-block-themes",
  "wp-interactivity-api",
  "wp-performance",
  "wp-phpstan",
  "wp-playground",
  "wp-plugin-development",
  "wp-plugin-directory-guidelines",
  "wp-project-triage",
  "wp-rest-api",
  "wp-wpcli-and-ops",
  "wpds",
]

[presets]
lean = []
web = []
go = ["go"]
wordpress = ["wordpress"]
`, home, home, home, home, home, home, home, home, home, home, home, home))
}

func MinimalConfigTOML() string {
	home := "$HOME"
	return strings.TrimSpace(fmt.Sprintf(`
# agentswitch config
#
# This minimal config defines the standard user skill roots and empty pack /
# preset sections. Add packs and presets for your own workflow.

[[roots]]
name = "user"
active = "%s/.agents/skills"
disabled = "%s/.agents/skills.disabled"
switchable = true

[[roots]]
name = "admin"
active = "/etc/codex/skills"
switchable = false

[presets]
`, home, home))
}

func ParseConfig(input string) (Config, error) {
	config := Config{
		Packs:   map[string]PackLayout{},
		Presets: map[string][]string{},
	}
	var section string
	var currentRoot *RootPair
	var currentPackName string
	var currentArrayKey string
	var currentArray []string

	flushArray := func() error {
		if currentArrayKey == "" {
			return nil
		}
		switch {
		case section == "presets":
			config.Presets[currentArrayKey] = append([]string(nil), currentArray...)
		case strings.HasPrefix(section, "packs."):
			pack := config.Packs[currentPackName]
			if currentArrayKey != "skills" {
				return fmt.Errorf("unsupported array key %q in [%s]", currentArrayKey, section)
			}
			pack.Skills = append([]string(nil), currentArray...)
			config.Packs[currentPackName] = pack
		default:
			return fmt.Errorf("array key %q is not valid in [%s]", currentArrayKey, section)
		}
		currentArrayKey = ""
		currentArray = nil
		return nil
	}

	lines := strings.Split(input, "\n")
	for lineNumber, raw := range lines {
		line := stripComment(strings.TrimSpace(raw))
		if line == "" {
			continue
		}

		if currentArrayKey != "" {
			done, values, err := parseArrayContinuation(line)
			if err != nil {
				return Config{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
			}
			currentArray = append(currentArray, values...)
			if done {
				if err := flushArray(); err != nil {
					return Config{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
				}
			}
			continue
		}

		if line == "[[roots]]" {
			section = "roots"
			config.Roots = append(config.Roots, RootPair{})
			currentRoot = &config.Roots[len(config.Roots)-1]
			currentPackName = ""
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			currentRoot = nil
			currentPackName = ""
			if strings.HasPrefix(section, "packs.") {
				currentPackName = strings.TrimPrefix(section, "packs.")
				if currentPackName == "" {
					return Config{}, fmt.Errorf("line %d: empty pack section", lineNumber+1)
				}
				if _, ok := config.Packs[currentPackName]; !ok {
					config.Packs[currentPackName] = PackLayout{}
				}
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key = value", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if strings.HasPrefix(value, "[") && !strings.Contains(value, "]") {
			currentArrayKey = key
			currentArray = nil
			continue
		}

		if strings.HasPrefix(value, "[") {
			values, err := parseStringArray(value)
			if err != nil {
				return Config{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
			}
			switch {
			case section == "presets":
				config.Presets[key] = values
			case strings.HasPrefix(section, "packs.") && key == "skills":
				pack := config.Packs[currentPackName]
				pack.Skills = values
				config.Packs[currentPackName] = pack
			default:
				return Config{}, fmt.Errorf("line %d: unsupported array key %q in [%s]", lineNumber+1, key, section)
			}
			continue
		}

		switch section {
		case "roots":
			if currentRoot == nil {
				return Config{}, fmt.Errorf("line %d: root value outside [[roots]]", lineNumber+1)
			}
			if err := setRootValue(currentRoot, key, value); err != nil {
				return Config{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
			}
		default:
			if strings.HasPrefix(section, "packs.") {
				pack := config.Packs[currentPackName]
				if err := setPackValue(&pack, key, value); err != nil {
					return Config{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
				}
				config.Packs[currentPackName] = pack
				continue
			}
			return Config{}, fmt.Errorf("line %d: unsupported section [%s]", lineNumber+1, section)
		}
	}
	if err := flushArray(); err != nil {
		return Config{}, err
	}
	expandConfigPaths(&config)
	return config, nil
}

func setRootValue(root *RootPair, key string, value string) error {
	switch key {
	case "name":
		root.Name = parseString(value)
	case "active":
		root.Active = parseString(value)
	case "disabled":
		root.Disabled = parseString(value)
	case "switchable":
		root.Switchable = value == "true"
	default:
		return fmt.Errorf("unsupported root key %q", key)
	}
	return nil
}

func setPackValue(pack *PackLayout, key string, value string) error {
	switch key {
	case "type":
		pack.Type = parseString(value)
	case "active":
		pack.Active = parseString(value)
	case "disabled":
		pack.Disabled = parseString(value)
	case "skills_subdir":
		pack.SkillsSubdir = parseString(value)
	case "link_root":
		pack.LinkRoot = parseString(value)
	case "disabled_fallback":
		pack.DisabledFallback = parseString(value)
	default:
		return fmt.Errorf("unsupported pack key %q", key)
	}
	return nil
}

func expandConfigPaths(config *Config) {
	for i := range config.Roots {
		config.Roots[i].Active = expandPath(config.Roots[i].Active)
		config.Roots[i].Disabled = expandPath(config.Roots[i].Disabled)
	}
	for name, pack := range config.Packs {
		pack.Active = expandPath(pack.Active)
		pack.Disabled = expandPath(pack.Disabled)
		pack.LinkRoot = expandPath(pack.LinkRoot)
		pack.DisabledFallback = expandPath(pack.DisabledFallback)
		config.Packs[name] = pack
	}
}

func expandPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" || path == "$HOME" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if strings.HasPrefix(path, "$HOME/") {
		return filepath.Join(home, strings.TrimPrefix(path, "$HOME/"))
	}
	return path
}

func stripComment(line string) string {
	inString := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inString = !inString
		case '#':
			if !inString {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return line
}

func parseString(value string) string {
	value = strings.TrimSpace(value)
	return strings.Trim(value, `"`)
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("expected string array")
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if body == "" {
		return []string{}, nil
	}
	parts := strings.Split(body, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, parseString(part))
	}
	return values, nil
}

func parseArrayContinuation(line string) (bool, []string, error) {
	done := strings.Contains(line, "]")
	line = strings.TrimSpace(strings.TrimSuffix(line, "]"))
	line = strings.TrimSuffix(line, ",")
	if line == "" {
		return done, nil, nil
	}
	values, err := parseStringArray("[" + line + "]")
	return done, values, err
}

func ConfiguredPackNames(config Config) []string {
	names := make([]string, 0, len(config.Packs))
	for name := range config.Packs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
