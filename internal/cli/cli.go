package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/srmdn/agentswitch/internal/skills"
)

const restartReminder = "Restart Codex for skill changes to take effect."

type app struct {
	out    io.Writer
	errOut io.Writer
	dryRun bool
}

func Run(args []string, out, errOut io.Writer) int {
	a := &app{out: out, errOut: errOut}
	if err := a.run(args); err != nil {
		fmt.Fprintln(errOut, "agentswitch:", err)
		return 1
	}
	return 0
}

func (a *app) run(args []string) error {
	fs := flag.NewFlagSet("agentswitch", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	fs.BoolVar(&a.dryRun, "dry-run", false, "show planned changes without modifying files")
	fs.BoolVar(&a.dryRun, "n", false, "alias for --dry-run")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 {
		a.printUsage()
		return nil
	}

	switch rest[0] {
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	case "init":
		return a.initConfig(rest[1:])
	case "config":
		return a.config(rest[1:])
	case "status":
		return a.skillsStatus()
	case "doctor":
		return a.skillsDoctor()
	case "enable":
		return a.skillSwitch(rest[1:], true)
	case "disable":
		return a.skillSwitch(rest[1:], false)
	case "pack":
		return a.pack(rest[1:])
	case "preset":
		return a.preset(rest[1:])
	case "skills":
		return a.skills(rest[1:])
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func (a *app) initConfig(args []string) error {
	force := false
	minimal := false
	for _, arg := range args {
		switch arg {
		case "--force":
			force = true
		case "--minimal":
			minimal = true
		default:
			return fmt.Errorf("unknown init option %q", arg)
		}
	}

	path := skills.DefaultConfigPath()
	if err := skills.InitConfig(path, force, minimal); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Created %s\n", path)
	return nil
}

func (a *app) config(args []string) error {
	if len(args) == 0 {
		return errors.New("missing config command")
	}

	switch args[0] {
	case "path":
		fmt.Fprintln(a.out, skills.DefaultConfigPath())
		return nil
	case "show":
		data, err := os.ReadFile(skills.DefaultConfigPath())
		if err != nil {
			return err
		}
		fmt.Fprint(a.out, string(data))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Fprintln(a.out)
		}
		return nil
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func (a *app) skills(args []string) error {
	if len(args) == 0 {
		return errors.New("missing skills command")
	}

	switch args[0] {
	case "status":
		return a.skillsStatus()
	case "enable":
		return a.skillsSwitch(args[1:], true)
	case "disable":
		return a.skillsSwitch(args[1:], false)
	case "preset":
		return a.skillsPreset(args[1:])
	case "doctor":
		return a.skillsDoctor()
	default:
		return fmt.Errorf("unknown skills command %q", args[0])
	}
}

func (a *app) pack(args []string) error {
	if len(args) == 0 {
		return errors.New("missing pack command")
	}

	switch args[0] {
	case "list":
		return a.packList()
	case "enable":
		return a.packSwitch(args[1:], true)
	case "disable":
		return a.packSwitch(args[1:], false)
	default:
		return fmt.Errorf("unknown pack command %q", args[0])
	}
}

func (a *app) preset(args []string) error {
	if len(args) == 0 {
		return errors.New("missing preset command")
	}

	switch args[0] {
	case "list":
		return a.presetList()
	case "apply":
		return a.presetApply(args[1:])
	default:
		return fmt.Errorf("unknown preset command %q", args[0])
	}
}

func (a *app) skillsStatus() error {
	manager, err := a.manager()
	if err != nil {
		return err
	}
	inventory, err := manager.Inventory()
	if err != nil {
		return err
	}
	printInventory(a.out, inventory)
	return nil
}

func (a *app) skillsDoctor() error {
	manager, err := a.manager()
	if err != nil {
		return err
	}
	inventory, err := manager.Inventory()
	if err != nil {
		return err
	}
	broken := inventory.Broken()
	if len(broken) == 0 {
		fmt.Fprintln(a.out, "No broken skill symlinks found.")
		return nil
	}
	fmt.Fprintln(a.out, "Broken skill symlinks:")
	for _, skill := range broken {
		fmt.Fprintf(a.out, "- %s (%s)\n", skill.Name, skill.Path)
	}
	return nil
}

func (a *app) packList() error {
	m, err := a.manager()
	if err != nil {
		return err
	}
	names := m.PackNames()
	if len(names) == 0 {
		fmt.Fprintln(a.out, "No packs configured.")
		return nil
	}
	for _, name := range names {
		packSkills, _ := m.PackSkills(name)
		fmt.Fprintf(a.out, "%s: %s\n", name, strings.Join(packSkills, ", "))
	}
	return nil
}

func (a *app) presetList() error {
	m, err := a.manager()
	if err != nil {
		return err
	}
	names := m.PresetNames()
	if len(names) == 0 {
		fmt.Fprintln(a.out, "No presets configured.")
		return nil
	}
	for _, name := range names {
		fmt.Fprintln(a.out, name)
	}
	return nil
}

func (a *app) skillSwitch(args []string, enable bool) error {
	if len(args) != 1 {
		if enable {
			return errors.New("usage: agentswitch enable <skill>")
		}
		return errors.New("usage: agentswitch disable <skill>")
	}

	m, err := a.manager()
	if err != nil {
		return err
	}
	changes, err := m.SetSkillEnabled(args[0], enable, a.dryRun)
	if err != nil {
		return err
	}
	printChanges(a.out, changes, a.dryRun)
	if len(changes) > 0 && !a.dryRun {
		fmt.Fprintln(a.out, restartReminder)
	}
	return nil
}

func (a *app) packSwitch(args []string, enable bool) error {
	if len(args) != 1 {
		if enable {
			return errors.New("usage: agentswitch pack enable <pack>")
		}
		return errors.New("usage: agentswitch pack disable <pack>")
	}

	m, err := a.manager()
	if err != nil {
		return err
	}
	changes, err := m.SetPackEnabled(args[0], enable, a.dryRun)
	if err != nil {
		return err
	}
	printChanges(a.out, changes, a.dryRun)
	if len(changes) > 0 && !a.dryRun {
		fmt.Fprintln(a.out, restartReminder)
	}
	return nil
}

func (a *app) skillsSwitch(args []string, enable bool) error {
	if len(args) != 1 {
		if enable {
			return errors.New("usage: agentswitch skills enable <name-or-pack>")
		}
		return errors.New("usage: agentswitch skills disable <name-or-pack>")
	}

	m, err := a.manager()
	if err != nil {
		return err
	}
	changes, err := m.SetEnabled(args[0], enable, a.dryRun)
	if err != nil {
		return err
	}
	printChanges(a.out, changes, a.dryRun)
	if len(changes) > 0 && !a.dryRun {
		fmt.Fprintln(a.out, restartReminder)
	}
	return nil
}

func (a *app) presetApply(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: agentswitch preset apply <preset>")
	}

	m, err := a.manager()
	if err != nil {
		return err
	}
	changes, err := m.ApplyPreset(args[0], a.dryRun)
	if err != nil {
		return err
	}
	printChanges(a.out, changes, a.dryRun)
	if len(changes) > 0 && !a.dryRun {
		fmt.Fprintln(a.out, restartReminder)
	}
	return nil
}

func (a *app) skillsPreset(args []string) error {
	return a.presetApply(args)
}

func (a *app) manager() (skills.Manager, error) {
	return skills.LoadDefaultManager()
}

func (a *app) printUsage() {
	fmt.Fprintln(a.out, strings.TrimSpace(`
agentswitch manages active and disabled agent skills.

Usage:
  agentswitch status
  agentswitch init
  agentswitch config path
  agentswitch config show
  agentswitch enable <skill>
  agentswitch disable <skill>
  agentswitch pack list
  agentswitch pack enable <pack>
  agentswitch pack disable <pack>
  agentswitch preset list
  agentswitch preset apply <preset>
  agentswitch doctor

Compatibility:
  agentswitch skills status
  agentswitch skills enable <name-or-pack>
  agentswitch skills disable <name-or-pack>
  agentswitch skills preset <preset>
  agentswitch skills doctor

Flags:
  --dry-run, -n  show planned changes without modifying files

Init:
  agentswitch init            create example ~/.config/agentswitch/config.toml
  agentswitch init --minimal  create minimal ~/.config/agentswitch/config.toml
  agentswitch init --force    overwrite ~/.config/agentswitch/config.toml
`))
}

func printInventory(out io.Writer, inventory skills.Inventory) {
	if len(inventory.Items) == 0 {
		fmt.Fprintln(out, "No skills found.")
		return
	}

	fmt.Fprintln(out, "STATE     ROOT    NAME")
	for _, skill := range inventory.Items {
		state := "active"
		if !skill.Active {
			state = "disabled"
		}
		if skill.BrokenSymlink {
			state += "!"
		}
		fmt.Fprintf(out, "%-9s %-7s %s\n", state, skill.RootName, skill.Name)
	}
}

func printChanges(out io.Writer, changes []skills.Change, dryRun bool) {
	if len(changes) == 0 {
		fmt.Fprintln(out, "No changes.")
		return
	}

	prefix := ""
	if dryRun {
		prefix = "would "
	}
	for _, change := range changes {
		fmt.Fprintf(out, "%s%s %s\n", prefix, change.Action, change.Name)
	}
}
