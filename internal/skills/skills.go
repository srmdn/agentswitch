package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type RootPair struct {
	Name       string
	Active     string
	Disabled   string
	Switchable bool
}

type Manager struct {
	Roots       []RootPair
	Packs       map[string][]string
	PackLayouts map[string]PackLayout
	Presets     map[string][]string
	CompatPaths CompatPaths
}

type PackLayout struct {
	Type             string
	Skills           []string
	Active           string
	Disabled         string
	SkillsSubdir     string
	LinkRoot         string
	DisabledFallback string
}

type CompatPaths struct {
	AgentsSkills     string
	AgentsDisabled   string
	CodexSkills      string
	CodexDisabled    string
	GoPack           string
	WordPressPack    string
	TranslationGroup string
}

type Skill struct {
	Name          string
	Path          string
	RootName      string
	RootIndex     int
	Active        bool
	BrokenSymlink bool
}

type Inventory struct {
	Items []Skill
}

type Summary struct {
	Active   int
	Disabled int
	Broken   int
	Total    int
}

type Change struct {
	Action string
	Name   string
	From   string
	To     string
}

func DefaultManager() Manager {
	manager, err := LoadDefaultManager()
	if err != nil {
		return DefaultManagerWithoutConfig()
	}
	return manager
}

func LoadDefaultManager() (Manager, error) {
	manager := DefaultManagerWithoutConfig()
	configPath := DefaultConfigPath()
	config, err := LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manager, nil
		}
		return Manager{}, fmt.Errorf("load config %s: %w", configPath, err)
	}
	manager.ApplyConfig(config)
	return manager, nil
}

func DefaultManagerWithoutConfig() Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "~"
	}

	agentsSkills := filepath.Join(home, ".agents", "skills")
	agentsDisabled := filepath.Join(home, ".agents", "skills.disabled")
	codexSkills := filepath.Join(home, ".codex", "skills")
	codexDisabled := filepath.Join(home, ".codex", "skills.disabled")

	roots := repoSkillRoots(".")
	roots = append(roots,
		RootPair{
			Name:       "user",
			Active:     agentsSkills,
			Disabled:   agentsDisabled,
			Switchable: true,
		},
		RootPair{
			Name:       "codex",
			Active:     codexSkills,
			Disabled:   codexDisabled,
			Switchable: true,
		},
		RootPair{
			Name:   "admin",
			Active: "/etc/codex/skills",
		},
	)

	return Manager{
		Roots:       roots,
		Packs:       map[string][]string{},
		PackLayouts: map[string]PackLayout{},
		Presets:     map[string][]string{},
		CompatPaths: CompatPaths{
			AgentsSkills:     agentsSkills,
			AgentsDisabled:   agentsDisabled,
			CodexSkills:      codexSkills,
			CodexDisabled:    codexDisabled,
			GoPack:           "cc-skills-golang",
			WordPressPack:    "wordpress-pack",
			TranslationGroup: "translation",
		},
	}
}

func (m *Manager) ApplyConfig(config Config) {
	if len(config.Roots) > 0 {
		m.Roots = append(repoSkillRoots("."), config.Roots...)
	}
	if config.Packs != nil {
		m.Packs = map[string][]string{}
		m.PackLayouts = config.Packs
	}
	if config.Presets != nil {
		m.Presets = config.Presets
	}
}

func (m Manager) Inventory() (Inventory, error) {
	var items []Skill
	for i, root := range m.Roots {
		active, err := scanRoot(root.Active, root.Name, i, true)
		if err != nil {
			return Inventory{}, err
		}
		if root.Disabled != "" {
			disabled, err := scanRoot(root.Disabled, root.Name, i, false)
			if err != nil {
				return Inventory{}, err
			}
			items = append(items, disabled...)
		}
		items = append(items, active...)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].RootName != items[j].RootName {
			return items[i].RootName < items[j].RootName
		}
		if items[i].Active != items[j].Active {
			return items[i].Active
		}
		return items[i].Name < items[j].Name
	})

	return Inventory{Items: items}, nil
}

func (i Inventory) Broken() []Skill {
	var broken []Skill
	for _, skill := range i.Items {
		if skill.BrokenSymlink {
			broken = append(broken, skill)
		}
	}
	return broken
}

func (i Inventory) Summary() Summary {
	var summary Summary
	for _, skill := range i.Items {
		summary.Total++
		if skill.Active {
			summary.Active++
		} else {
			summary.Disabled++
		}
		if skill.BrokenSymlink {
			summary.Broken++
		}
	}
	return summary
}

func (m Manager) PackNames() []string {
	seen := map[string]bool{}
	for name := range m.Packs {
		seen[name] = true
	}
	for name := range m.PackLayouts {
		seen[name] = true
	}
	return sortedBoolKeys(seen)
}

func (m Manager) PresetNames() []string {
	return sortedKeys(m.Presets)
}

func (m Manager) PackSkills(name string) ([]string, bool) {
	if layout, ok := m.PackLayouts[name]; ok {
		return m.layoutPackSkills(layout)
	}
	if names, ok := m.compatPackSkills(name); ok {
		return names, true
	}
	names, ok := m.Packs[name]
	if !ok {
		return nil, false
	}
	return append([]string(nil), names...), true
}

func (m Manager) SetSkillEnabled(name string, enabled bool, dryRun bool) ([]Change, error) {
	change, changed, err := m.setOne(name, enabled, dryRun)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return []Change{change}, nil
}

func (m Manager) SetPackEnabled(name string, enabled bool, dryRun bool) ([]Change, error) {
	if layout, ok := m.PackLayouts[name]; ok {
		return m.setLayoutPackEnabled(layout, enabled, dryRun)
	}
	if changes, handled, err := m.setCompatPackEnabled(name, enabled, dryRun); handled || err != nil {
		return changes, err
	}

	names, ok := m.PackSkills(name)
	if !ok {
		return nil, fmt.Errorf("unknown pack %q", name)
	}
	var changes []Change
	for _, name := range names {
		change, changed, err := m.setOne(name, enabled, dryRun)
		if err != nil {
			return changes, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func (m Manager) SetEnabled(nameOrPack string, enabled bool, dryRun bool) ([]Change, error) {
	if _, ok := m.Packs[nameOrPack]; ok {
		return m.SetPackEnabled(nameOrPack, enabled, dryRun)
	}
	return m.SetSkillEnabled(nameOrPack, enabled, dryRun)
}

func (m Manager) ApplyPreset(name string, dryRun bool) ([]Change, error) {
	activePacks, ok := m.Presets[name]
	if !ok {
		return nil, fmt.Errorf("unknown preset %q", name)
	}

	allowedPacks := map[string]bool{}
	for _, pack := range activePacks {
		allowedPacks[pack] = true
	}

	var changes []Change
	for _, pack := range m.PackNames() {
		next, err := m.SetPackEnabled(pack, allowedPacks[pack], dryRun)
		if err != nil {
			return changes, err
		}
		changes = append(changes, next...)
	}
	return changes, nil
}

func (m Manager) expandPack(name string) []string {
	if skills, ok := m.Packs[name]; ok {
		return append([]string(nil), skills...)
	}
	return []string{name}
}

func (m Manager) managedSkillNames() []string {
	seen := map[string]bool{}
	for _, skills := range m.Packs {
		for _, skill := range skills {
			seen[skill] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Manager) layoutPackSkills(layout PackLayout) ([]string, bool) {
	var names []string
	switch layout.Type {
	case "directory":
		names = uniqueSkillNames([]string{
			filepath.Join(layout.Active, layout.SkillsSubdir),
			filepath.Join(layout.Disabled, layout.SkillsSubdir),
		})
	case "group":
		names = uniqueSkillNames([]string{layout.Active, layout.Disabled, layout.DisabledFallback})
		names = filterNames(names, layout.Skills)
	case "symlinked":
		names = uniqueSkillNames([]string{layout.Active, layout.Disabled})
		names = filterPackNames("", names, layout.Skills)
	default:
		names = append([]string(nil), layout.Skills...)
	}
	if len(names) == 0 {
		return nil, false
	}
	return names, true
}

func (m Manager) setLayoutPackEnabled(layout PackLayout, enabled bool, dryRun bool) ([]Change, error) {
	switch layout.Type {
	case "directory":
		return setDirectoryPackEnabled(layout, enabled, dryRun)
	case "group":
		return setGroupPackEnabled(layout, enabled, dryRun)
	case "symlinked":
		return setSymlinkedPackEnabled(layout, enabled, dryRun)
	default:
		var changes []Change
		for _, name := range layout.Skills {
			change, changed, err := m.setOneAllowMissing(name, enabled, dryRun, true)
			if err != nil {
				return changes, err
			}
			if changed {
				changes = append(changes, change)
			}
		}
		return changes, nil
	}
}

func setDirectoryPackEnabled(layout PackLayout, enabled bool, dryRun bool) ([]Change, error) {
	from, to := layout.Active, layout.Disabled
	action := "disabled"
	if enabled {
		from, to = layout.Disabled, layout.Active
		action = "enabled"
	}
	if !pathExists(from) {
		return nil, nil
	}
	changes := changesForNames(action, uniqueSkillNames([]string{filepath.Join(from, layout.SkillsSubdir)}), from, to)
	if dryRun {
		return changes, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, err
	}
	if err := movePath(from, to); err != nil {
		return nil, err
	}
	return changes, nil
}

func setGroupPackEnabled(layout PackLayout, enabled bool, dryRun bool) ([]Change, error) {
	names := filterNames(uniqueSkillNames([]string{layout.Active, layout.Disabled, layout.DisabledFallback}), layout.Skills)
	var changes []Change
	for _, name := range names {
		var from string
		to := filepath.Join(layout.Disabled, name)
		action := "disabled"
		if enabled {
			from = firstExistingPath(filepath.Join(layout.Disabled, name), filepath.Join(layout.DisabledFallback, name))
			to = filepath.Join(layout.Active, name)
			action = "enabled"
		} else {
			from = filepath.Join(layout.Active, name)
		}
		if from == "" || !pathExists(from) {
			continue
		}
		changes = append(changes, Change{Action: action, Name: name, From: from, To: to})
		if dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return changes, err
		}
		if err := movePath(from, to); err != nil {
			return changes, err
		}
	}
	return changes, nil
}

func setSymlinkedPackEnabled(layout PackLayout, enabled bool, dryRun bool) ([]Change, error) {
	names := filterPackNames("", uniqueSkillNames([]string{layout.Active, layout.Disabled}), layout.Skills)
	var changes []Change
	for _, name := range names {
		activeSkill := filepath.Join(layout.Active, name)
		disabledSkill := filepath.Join(layout.Disabled, name)
		link := filepath.Join(layout.LinkRoot, name)

		if enabled {
			if pathExists(disabledSkill) {
				changes = append(changes, Change{Action: "enabled", Name: name, From: disabledSkill, To: activeSkill})
				if !dryRun {
					if err := os.MkdirAll(layout.Active, 0o755); err != nil {
						return changes, err
					}
					if err := movePath(disabledSkill, activeSkill); err != nil {
						return changes, err
					}
				}
			}
			if pathExists(activeSkill) && !pathExists(link) {
				changes = append(changes, Change{Action: "linked", Name: name, From: activeSkill, To: link})
				if !dryRun {
					if err := os.MkdirAll(layout.LinkRoot, 0o755); err != nil {
						return changes, err
					}
					if err := os.Symlink(activeSkill, link); err != nil {
						return changes, err
					}
				}
			}
			continue
		}

		if isSymlink(link) {
			changes = append(changes, Change{Action: "unlinked", Name: name, From: link})
			if !dryRun {
				if err := os.Remove(link); err != nil {
					return changes, err
				}
			}
		}
		if pathExists(activeSkill) {
			changes = append(changes, Change{Action: "disabled", Name: name, From: activeSkill, To: disabledSkill})
			if !dryRun {
				if err := os.MkdirAll(layout.Disabled, 0o755); err != nil {
					return changes, err
				}
				if err := movePath(activeSkill, disabledSkill); err != nil {
					return changes, err
				}
			}
		}
	}
	return changes, nil
}

func (m Manager) compatPackSkills(name string) ([]string, bool) {
	var dirs []string
	switch name {
	case "go":
		dirs = []string{
			filepath.Join(m.CompatPaths.AgentsSkills, m.CompatPaths.GoPack, "skills"),
			filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.GoPack, "skills"),
		}
	case "translation":
		names := uniqueSkillNames([]string{
			m.CompatPaths.AgentsSkills,
			filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.TranslationGroup),
			m.CompatPaths.AgentsDisabled,
		})
		return filterNames(names, m.Packs["translation"]), true
	case "wordpress":
		names := uniqueSkillNames([]string{
			m.CompatPaths.CodexSkills,
			filepath.Join(m.CompatPaths.CodexDisabled, m.CompatPaths.WordPressPack),
		})
		filtered := filterPackNames(name, names, m.Packs["wordpress"])
		if len(filtered) == 0 {
			return nil, false
		}
		return filtered, true
	default:
		return nil, false
	}

	names := uniqueSkillNames(dirs)
	if len(names) == 0 {
		return nil, false
	}
	return names, true
}

func (m Manager) setCompatPackEnabled(name string, enabled bool, dryRun bool) ([]Change, bool, error) {
	switch name {
	case "go":
		return m.setGoPackEnabled(enabled, dryRun)
	case "translation":
		return m.setTranslationPackEnabled(enabled, dryRun)
	case "wordpress":
		return m.setWordPressPackEnabled(enabled, dryRun)
	default:
		return nil, false, nil
	}
}

func (m Manager) setGoPackEnabled(enabled bool, dryRun bool) ([]Change, bool, error) {
	active := filepath.Join(m.CompatPaths.AgentsSkills, m.CompatPaths.GoPack)
	disabled := filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.GoPack)
	if !pathExists(active) && !pathExists(disabled) {
		return nil, false, nil
	}
	from, to := active, disabled
	action := "disabled"
	if enabled {
		from, to = disabled, active
		action = "enabled"
	}

	if !pathExists(from) {
		return nil, true, nil
	}
	changes := changesForNames(action, uniqueSkillNames([]string{filepath.Join(from, "skills")}), from, to)
	if dryRun {
		return changes, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, true, err
	}
	if err := movePath(from, to); err != nil {
		return nil, true, err
	}
	return changes, true, nil
}

func (m Manager) setTranslationPackEnabled(enabled bool, dryRun bool) ([]Change, bool, error) {
	names := filterNames(uniqueSkillNames([]string{
		m.CompatPaths.AgentsSkills,
		filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.TranslationGroup),
		m.CompatPaths.AgentsDisabled,
	}), m.Packs["translation"])
	if len(names) == 0 {
		return nil, false, nil
	}

	var changes []Change
	for _, name := range names {
		var from string
		to := filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.TranslationGroup, name)
		action := "disabled"
		if enabled {
			from = firstExistingPath(
				filepath.Join(m.CompatPaths.AgentsDisabled, m.CompatPaths.TranslationGroup, name),
				filepath.Join(m.CompatPaths.AgentsDisabled, name),
			)
			to = filepath.Join(m.CompatPaths.AgentsSkills, name)
			action = "enabled"
		} else {
			from = filepath.Join(m.CompatPaths.AgentsSkills, name)
		}
		if from == "" || !pathExists(from) {
			continue
		}
		changes = append(changes, Change{Action: action, Name: name, From: from, To: to})
		if dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return changes, true, err
		}
		if err := movePath(from, to); err != nil {
			return changes, true, err
		}
	}
	return changes, true, nil
}

func (m Manager) setWordPressPackEnabled(enabled bool, dryRun bool) ([]Change, bool, error) {
	packDir := filepath.Join(m.CompatPaths.CodexDisabled, m.CompatPaths.WordPressPack)
	names := filterPackNames("wordpress", uniqueSkillNames([]string{m.CompatPaths.CodexSkills, packDir}), m.Packs["wordpress"])
	if len(names) == 0 {
		return nil, false, nil
	}

	var changes []Change
	for _, name := range names {
		activeSkill := filepath.Join(m.CompatPaths.CodexSkills, name)
		disabledSkill := filepath.Join(packDir, name)
		link := filepath.Join(m.CompatPaths.AgentsSkills, name)

		if enabled {
			if pathExists(disabledSkill) {
				changes = append(changes, Change{Action: "enabled", Name: name, From: disabledSkill, To: activeSkill})
				if !dryRun {
					if err := os.MkdirAll(m.CompatPaths.CodexSkills, 0o755); err != nil {
						return changes, true, err
					}
					if err := movePath(disabledSkill, activeSkill); err != nil {
						return changes, true, err
					}
				}
			}
			if pathExists(activeSkill) && !pathExists(link) {
				changes = append(changes, Change{Action: "linked", Name: name, From: activeSkill, To: link})
				if !dryRun {
					if err := os.MkdirAll(m.CompatPaths.AgentsSkills, 0o755); err != nil {
						return changes, true, err
					}
					if err := os.Symlink(activeSkill, link); err != nil {
						return changes, true, err
					}
				}
			}
			continue
		}

		if isSymlink(link) {
			changes = append(changes, Change{Action: "unlinked", Name: name, From: link})
			if !dryRun {
				if err := os.Remove(link); err != nil {
					return changes, true, err
				}
			}
		}
		if pathExists(activeSkill) {
			changes = append(changes, Change{Action: "disabled", Name: name, From: activeSkill, To: disabledSkill})
			if !dryRun {
				if err := os.MkdirAll(packDir, 0o755); err != nil {
					return changes, true, err
				}
				if err := movePath(activeSkill, disabledSkill); err != nil {
					return changes, true, err
				}
			}
		}
	}
	return changes, true, nil
}

func sortedKeys(values map[string][]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedBoolKeys(values map[string]bool) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Manager) setOne(name string, enabled bool, dryRun bool) (Change, bool, error) {
	return m.setOneAllowMissing(name, enabled, dryRun, false)
}

func (m Manager) setOneAllowMissing(name string, enabled bool, dryRun bool, allowMissing bool) (Change, bool, error) {
	inventory, err := m.Inventory()
	if err != nil {
		return Change{}, false, err
	}

	for _, skill := range inventory.Items {
		root := m.Roots[skill.RootIndex]
		if skill.Name == name && skill.Active == enabled && root.Switchable {
			return Change{}, false, nil
		}
	}

	for _, skill := range inventory.Items {
		if skill.Name != name || skill.Active == enabled {
			continue
		}

		root := m.Roots[skill.RootIndex]
		if !root.Switchable {
			continue
		}
		destinationRoot := root.Disabled
		action := "disabled"
		if enabled {
			destinationRoot = root.Active
			action = "enabled"
		}
		destination := filepath.Join(destinationRoot, filepath.Base(skill.Path))
		change := Change{Action: action, Name: skill.Name, From: skill.Path, To: destination}

		if dryRun {
			return change, true, nil
		}
		if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
			return Change{}, false, err
		}
		if err := movePath(skill.Path, destination); err != nil {
			return Change{}, false, err
		}
		return change, true, nil
	}

	if allowMissing {
		return Change{}, false, nil
	}
	return Change{}, false, fmt.Errorf("switchable skill %q not found", name)
}

func scanRoot(root string, rootName string, rootIndex int, active bool) ([]Skill, error) {
	var skills []Skill
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		info, err := os.Lstat(path)
		if err != nil {
			return err
		}

		isSymlink := info.Mode()&os.ModeSymlink != 0
		if isSymlink {
			if _, err := filepath.EvalSymlinks(path); err != nil {
				if filepath.Dir(path) == root {
					skills = append(skills, Skill{
						Name:          entry.Name(),
						Path:          path,
						RootName:      rootName,
						RootIndex:     rootIndex,
						Active:        active,
						BrokenSymlink: true,
					})
				}
				return nil
			}
			if hasSkillFile(path) {
				skills = append(skills, Skill{
					Name:      entry.Name(),
					Path:      path,
					RootName:  rootName,
					RootIndex: rootIndex,
					Active:    active,
				})
			}
			return nil
		}

		if entry.IsDir() && hasSkillFile(path) {
			skills = append(skills, Skill{
				Name:      entry.Name(),
				Path:      path,
				RootName:  rootName,
				RootIndex: rootIndex,
				Active:    active,
			})
			return filepath.SkipDir
		}

		if entry.Name() != "SKILL.md" {
			return nil
		}
		skillDir := filepath.Dir(path)
		skills = append(skills, Skill{
			Name:      filepath.Base(skillDir),
			Path:      skillDir,
			RootName:  rootName,
			RootIndex: rootIndex,
			Active:    active,
		})
		return filepath.SkipDir
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return skills, nil
}

func hasSkillFile(path string) bool {
	info, err := os.Stat(filepath.Join(path, "SKILL.md"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func repoSkillRoots(start string) []RootPair {
	absStart, err := filepath.Abs(start)
	if err != nil {
		absStart = start
	}
	repoRoot := findRepoRoot(absStart)

	var roots []RootPair
	for dir := absStart; ; dir = filepath.Dir(dir) {
		name := "repo"
		if rel, err := filepath.Rel(repoRoot, dir); err == nil && rel != "." {
			name = "repo:" + rel
		}
		roots = append(roots, RootPair{
			Name:   name,
			Active: filepath.Join(dir, ".agents", "skills"),
		})

		if dir == repoRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return roots
}

func findRepoRoot(start string) string {
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
	}
}

func uniqueSkillNames(dirs []string) []string {
	seen := map[string]bool{}
	for _, dir := range dirs {
		skills, err := scanSkillDirs(dir)
		if err != nil {
			continue
		}
		for _, name := range skills {
			seen[name] = true
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func scanSkillDirs(root string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if hasSkillFile(path) {
				names = append(names, entry.Name())
			}
			return nil
		}
		if entry.IsDir() && hasSkillFile(path) {
			names = append(names, entry.Name())
			return filepath.SkipDir
		}
		if entry.Name() != "SKILL.md" {
			return nil
		}
		names = append(names, filepath.Base(filepath.Dir(path)))
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

func filterNames(names []string, allowed []string) []string {
	allow := map[string]bool{}
	for _, name := range allowed {
		allow[name] = true
	}
	var filtered []string
	for _, name := range names {
		if allow[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func filterPackNames(pack string, names []string, allowed []string) []string {
	if len(allowed) == 1 && allowed[0] == pack {
		return names
	}
	return filterNames(names, allowed)
}

func changesForNames(action string, names []string, from string, to string) []Change {
	changes := make([]Change, 0, len(names))
	for _, name := range names {
		changes = append(changes, Change{Action: action, Name: name, From: from, To: to})
	}
	return changes
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func movePath(from string, to string) error {
	if pathExists(to) {
		return fmt.Errorf("destination already exists: %s", to)
	}
	return os.Rename(from, to)
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		if pathExists(path) {
			return path
		}
	}
	return ""
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
