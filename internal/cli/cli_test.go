package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpShowsClearCommandModel(t *testing.T) {
	t.Setenv("AGENTSWITCH_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	var out bytes.Buffer
	code := Run([]string{"help"}, &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	text := out.String()
	for _, want := range []string{
		"agentswitch init",
		"agentswitch enable <skill>",
		"agentswitch pack enable <pack>",
		"agentswitch preset apply <preset>",
		"Compatibility:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected help to contain %q:\n%s", want, text)
		}
	}
}

func TestMalformedConfigFailsCommand(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.toml")
	if err := os.WriteFile(configPath, []byte("[unknown]\nvalue = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTSWITCH_CONFIG", configPath)

	var errOut bytes.Buffer
	code := Run([]string{"status"}, &bytes.Buffer{}, &errOut)
	if code == 0 {
		t.Fatal("expected malformed config to fail")
	}
	if !strings.Contains(errOut.String(), "unsupported section") {
		t.Fatalf("expected parse error, got: %s", errOut.String())
	}
}

func TestPackAndPresetList(t *testing.T) {
	t.Setenv("AGENTSWITCH_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "pack list", args: []string{"pack", "list"}, want: "configured"},
		{name: "preset list", args: []string{"preset", "list"}, want: "configured"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := Run(tt.args, &out, &bytes.Buffer{})
			if code != 0 {
				t.Fatalf("expected success, got %d", code)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("expected output to contain %q:\n%s", tt.want, out.String())
			}
		})
	}
}
