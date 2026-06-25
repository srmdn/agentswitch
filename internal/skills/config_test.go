package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfigLoadsRootsPacksAndPresets(t *testing.T) {
	config, err := ParseConfig(`
[[roots]]
name = "user"
active = "$HOME/.agents/skills"
disabled = "$HOME/.agents/skills.disabled"
switchable = true

[packs.go]
type = "directory"
active = "$HOME/.agents/skills/cc-skills-golang"
disabled = "$HOME/.agents/skills.disabled/cc-skills-golang"
skills_subdir = "skills"

[packs.wordpress]
type = "symlinked"
active = "$HOME/.codex/skills"
disabled = "$HOME/.codex/skills.disabled/wordpress-pack"
link_root = "$HOME/.agents/skills"
skills = [
  "wp-rest-api",
  "wp-playground",
]

[presets]
lean = []
go = ["go"]
wordpress = ["wordpress"]
`)
	if err != nil {
		t.Fatal(err)
	}

	if len(config.Roots) != 1 || config.Roots[0].Name != "user" || !config.Roots[0].Switchable {
		t.Fatalf("unexpected roots: %#v", config.Roots)
	}
	if config.Packs["go"].Type != "directory" || config.Packs["go"].SkillsSubdir != "skills" {
		t.Fatalf("unexpected go pack: %#v", config.Packs["go"])
	}
	if got := config.Packs["wordpress"].Skills; len(got) != 2 || got[0] != "wp-rest-api" {
		t.Fatalf("unexpected wordpress skills: %#v", got)
	}
	if got := config.Presets["go"]; len(got) != 1 || got[0] != "go" {
		t.Fatalf("unexpected preset: %#v", config.Presets)
	}
}

func TestInitConfigWritesDefaultConfig(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "config.toml")
	if err := InitConfig(path, false, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseConfig(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Packs["wordpress"]; !ok {
		t.Fatalf("expected wordpress pack in generated config: %#v", config.Packs)
	}
	if _, ok := config.Presets["lean"]; !ok {
		t.Fatalf("expected lean preset in generated config: %#v", config.Presets)
	}
}

func TestInitConfigWritesMinimalConfig(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "config.toml")
	if err := InitConfig(path, false, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config, err := ParseConfig(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Packs) != 0 {
		t.Fatalf("expected minimal config to have no packs, got %#v", config.Packs)
	}
	if len(config.Roots) == 0 {
		t.Fatal("expected minimal config to include roots")
	}
}
