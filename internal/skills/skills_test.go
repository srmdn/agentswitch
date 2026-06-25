package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInventoryScansActiveDisabledAndBrokenSymlinks(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, manager.Roots[0].Active, "go")
	mkdirSkill(t, manager.Roots[0].Disabled, "wordpress")
	makeBrokenSymlink(t, manager.Roots[0].Active, "missing")

	inventory, err := manager.Inventory()
	if err != nil {
		t.Fatal(err)
	}

	if len(inventory.Items) != 3 {
		t.Fatalf("expected 3 skills, got %d: %#v", len(inventory.Items), inventory.Items)
	}

	broken := inventory.Broken()
	if len(broken) != 1 || broken[0].Name != "missing" {
		t.Fatalf("expected missing symlink to be reported broken, got %#v", broken)
	}
}

func TestSetEnabledMovesSkillBetweenRoots(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, manager.Roots[0].Disabled, "go")

	changes, err := manager.SetEnabled("go", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "enabled" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.Roots[0].Active, "go", "SKILL.md")); err != nil {
		t.Fatalf("expected enabled skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.Roots[0].Disabled, "go")); !os.IsNotExist(err) {
		t.Fatalf("expected disabled skill to be moved, stat err: %v", err)
	}
}

func TestDryRunDoesNotMoveSkill(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, manager.Roots[0].Disabled, "go")

	changes, err := manager.SetEnabled("go", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected dry-run change, got %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.Roots[0].Disabled, "go", "SKILL.md")); err != nil {
		t.Fatalf("expected dry-run to leave skill disabled: %v", err)
	}
}

func TestApplyPresetDisablesManagedSkillsOutsidePreset(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, manager.Roots[0].Active, "go")
	mkdirSkill(t, manager.Roots[0].Active, "wordpress")

	changes, err := manager.ApplyPreset("go", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Name != "wordpress" || changes[0].Action != "disabled" {
		t.Fatalf("unexpected changes: %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.Roots[0].Active, "go", "SKILL.md")); err != nil {
		t.Fatalf("expected go to remain active: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.Roots[0].Disabled, "wordpress", "SKILL.md")); err != nil {
		t.Fatalf("expected wordpress to be disabled: %v", err)
	}
}

func testManager(temp string) Manager {
	return Manager{
		Roots: []RootPair{{
			Name:       "test",
			Active:     filepath.Join(temp, "skills"),
			Disabled:   filepath.Join(temp, "skills.disabled"),
			Switchable: true,
		}},
		Packs: map[string][]string{
			"go":          {"go"},
			"wordpress":   {"wordpress"},
			"translation": {"id-locale-translation", "id-technical-docs-translation", "natural-id-translation"},
		},
		Presets: map[string][]string{
			"lean": {},
			"go":   {"go"},
		},
		CompatPaths: CompatPaths{
			AgentsSkills:     filepath.Join(temp, "skills"),
			AgentsDisabled:   filepath.Join(temp, "skills.disabled"),
			CodexSkills:      filepath.Join(temp, "codex", "skills"),
			CodexDisabled:    filepath.Join(temp, "codex", "skills.disabled"),
			GoPack:           "cc-skills-golang",
			WordPressPack:    "wordpress-pack",
			TranslationGroup: "translation",
		},
	}
}

func mkdirSkill(t *testing.T, root string, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeBrokenSymlink(t *testing.T, root string, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation on Windows requires additional privileges")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
}

func TestRepoSkillRootsWalkFromCWDToRepoRoot(t *testing.T) {
	temp := t.TempDir()
	if err := os.Mkdir(filepath.Join(temp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(temp, "services", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	roots := repoSkillRoots(nested)
	if len(roots) != 3 {
		t.Fatalf("expected nested, parent, root skill roots, got %#v", roots)
	}
	if roots[0].Active != filepath.Join(nested, ".agents", "skills") {
		t.Fatalf("unexpected first root: %#v", roots[0])
	}
	if roots[2].Active != filepath.Join(temp, ".agents", "skills") {
		t.Fatalf("unexpected repo root: %#v", roots[2])
	}
	for _, root := range roots {
		if root.Switchable {
			t.Fatalf("repo roots should be discovered but not switchable: %#v", root)
		}
	}
}

func TestInventoryScansNestedPackSkills(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, filepath.Join(manager.Roots[0].Disabled, "cc-skills-golang", "skills"), "golang-cli")

	inventory, err := manager.Inventory()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 1 {
		t.Fatalf("expected one nested skill, got %#v", inventory.Items)
	}
	if inventory.Items[0].Name != "golang-cli" || inventory.Items[0].Active {
		t.Fatalf("unexpected nested skill: %#v", inventory.Items[0])
	}
}

func TestGoCompatPackMovesWholePackDirectory(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, filepath.Join(manager.CompatPaths.AgentsDisabled, manager.CompatPaths.GoPack, "skills"), "golang-cli")
	mkdirSkill(t, filepath.Join(manager.CompatPaths.AgentsDisabled, manager.CompatPaths.GoPack, "skills"), "golang-testing")

	changes, err := manager.SetPackEnabled("go", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected two skill changes, got %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.AgentsSkills, manager.CompatPaths.GoPack, "skills", "golang-cli", "SKILL.md")); err != nil {
		t.Fatalf("expected go pack to move active: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.AgentsDisabled, manager.CompatPaths.GoPack)); !os.IsNotExist(err) {
		t.Fatalf("expected disabled go pack to move away, stat err: %v", err)
	}
}

func TestTranslationCompatPackMovesKnownSkillsToGroup(t *testing.T) {
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, manager.CompatPaths.AgentsSkills, "id-locale-translation")
	mkdirSkill(t, manager.CompatPaths.AgentsSkills, "docs-writer")

	changes, err := manager.SetPackEnabled("translation", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Name != "id-locale-translation" {
		t.Fatalf("expected only known translation skill to move, got %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.AgentsDisabled, manager.CompatPaths.TranslationGroup, "id-locale-translation", "SKILL.md")); err != nil {
		t.Fatalf("expected translation skill to move to group: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.AgentsSkills, "docs-writer", "SKILL.md")); err != nil {
		t.Fatalf("expected unrelated active skill to stay put: %v", err)
	}
}

func TestWordPressCompatPackMovesCodexSkillsAndSymlinksAgents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation on Windows requires additional privileges")
	}
	temp := t.TempDir()
	manager := testManager(temp)
	mkdirSkill(t, filepath.Join(manager.CompatPaths.CodexDisabled, manager.CompatPaths.WordPressPack), "wp-rest-api")

	changes, err := manager.SetPackEnabled("wordpress", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected move and link changes, got %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.CodexSkills, "wp-rest-api", "SKILL.md")); err != nil {
		t.Fatalf("expected wordpress skill to move active: %v", err)
	}
	link := filepath.Join(manager.CompatPaths.AgentsSkills, "wp-rest-api")
	if !isSymlink(link) {
		t.Fatalf("expected agents symlink at %s", link)
	}

	changes, err = manager.SetPackEnabled("wordpress", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected unlink and disable changes, got %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(manager.CompatPaths.CodexDisabled, manager.CompatPaths.WordPressPack, "wp-rest-api", "SKILL.md")); err != nil {
		t.Fatalf("expected wordpress skill to move disabled: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("expected agents symlink to be removed, stat err: %v", err)
	}
}
