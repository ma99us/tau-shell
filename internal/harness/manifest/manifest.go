// Package manifest describes harnesses declaratively. A manifest is a YAML
// file that says how to launch, resume, stop and read one AI CLI harness.
// Anything that needs code (parsing a harness's private log format) is a
// named reader implemented elsewhere; a manifest only picks it by name.
//
// This package is a leaf: it imports nothing from the rest of TauShell so
// session, tools, harness, usage and the UI host can all depend on it.
package manifest

import (
	"time"
)

// SchemaVersion is the only manifest schema this build understands.
const SchemaVersion = 1

// Manifest is one harness definition.
type Manifest struct {
	Schema     int    `yaml:"schema" json:"-"`
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	Short      string `yaml:"short" json:"short"`
	Executable string `yaml:"executable" json:"executable"`
	// Order positions the harness in the UI lists; ties sort by ID.
	Order int `yaml:"order,omitempty" json:"-"`

	Probes       Probes       `yaml:"probes" json:"-"`
	Capabilities []Capability `yaml:"capabilities" json:"-"`
	Launch       Launch       `yaml:"launch" json:"-"`
	Session      Session      `yaml:"session" json:"-"`
	// ResumeCheck, Model, Activity, Title and PhoneAnswer name built-in readers;
	// an empty name means the harness does not offer that feature.
	ResumeCheck  string      `yaml:"resume_check,omitempty" json:"-"`
	GracefulExit []ExitStep  `yaml:"graceful_exit,omitempty" json:"-"`
	Widgets      *Widgets    `yaml:"widgets,omitempty" json:"-"`
	Cues         []Cue       `yaml:"cues,omitempty" json:"cues"`
	Model        string      `yaml:"model,omitempty" json:"-"`
	Activity     string      `yaml:"activity,omitempty" json:"activity,omitempty"`
	Title        string      `yaml:"title,omitempty" json:"-"`
	Usage        Usage       `yaml:"usage,omitempty" json:"-"`
	Skills       Skills      `yaml:"skills,omitempty" json:"-"`
	ConfigFile   *ConfigFile `yaml:"config_file,omitempty" json:"-"`
	PhoneAnswer  string      `yaml:"phone_answer,omitempty" json:"phoneAnswer,omitempty"`
	Install      Install     `yaml:"install" json:"-"`
	Brand        Brand       `yaml:"brand" json:"brand"`

	// Filled in by the loader, never read from YAML.
	Builtin bool   `yaml:"-" json:"builtin"`
	Path    string `yaml:"-" json:"path,omitempty"`
	Hash    string `yaml:"-" json:"hash,omitempty"`
}

type Probes struct {
	VersionArgs  []string `yaml:"version_args"`
	HelpArgs     []string `yaml:"help_args"`
	Minimum      string   `yaml:"minimum,omitempty"`
	TestedFrom   string   `yaml:"tested_from,omitempty"`
	TestedBefore string   `yaml:"tested_before,omitempty"`
}

type Capability struct {
	Name     string `yaml:"name"`
	Option   string `yaml:"option"`
	Optional bool   `yaml:"optional,omitempty"`
}

type Launch struct {
	// DefaultArgs is the editable launch_args preset for a new session.
	DefaultArgs []string `yaml:"default_args,omitempty"`
	// ReservedArgs are options users may not put in launch_args because
	// TauShell manages them. Matching ignores case and any "=value".
	ReservedArgs []string `yaml:"reserved_args,omitempty"`
}

// Session says how TauShell names and reopens a conversation.
type Session struct {
	// ID is "tau" when TauShell picks the conversation ID and passes it in,
	// or "discover" when the harness picks it and TauShell finds it later.
	ID string `yaml:"id"`
	// File is an optional path template that {session_file} expands to; its
	// folder is created before launch.
	File       string   `yaml:"file,omitempty"`
	StartArgs  []string `yaml:"start_args,omitempty"`
	ResumeArgs []string `yaml:"resume_args"`
	// ResumePos is "after" (default) or "before" the user's launch_args.
	ResumePos string `yaml:"resume_position,omitempty"`
	Discovery string `yaml:"discovery,omitempty"`
}

type ExitStep struct {
	Input   string `yaml:"input"`
	PauseMS int    `yaml:"pause_ms,omitempty"`
}

// Pause returns the step's pause as a duration.
func (step ExitStep) Pause() time.Duration {
	return time.Duration(step.PauseMS) * time.Millisecond
}

type Command struct {
	Label string `yaml:"label"`
	Text  string `yaml:"text"`
}

type Widgets struct {
	Commands      []Command `yaml:"commands,omitempty"`
	Models        []string  `yaml:"models,omitempty"`
	InterruptKeys []string  `yaml:"interrupt_keys,omitempty"`
	CompactText   string    `yaml:"compact_text,omitempty"`
	ClearText     string    `yaml:"clear_text,omitempty"`
}

// Cue is one ordered state rule. The first rule whose pattern matches wins.
type Cue struct {
	State      string `yaml:"state" json:"state"`
	Source     string `yaml:"source" json:"source"`
	Rows       int    `yaml:"rows,omitempty" json:"rows,omitempty"`
	Pattern    string `yaml:"pattern" json:"pattern"`
	IgnoreCase bool   `yaml:"ignore_case,omitempty" json:"ignoreCase,omitempty"`
}

type Usage struct {
	Reader         string `yaml:"reader,omitempty"`
	RecordsOwnCost bool   `yaml:"records_own_cost,omitempty"`
}

type Skills struct {
	// PluginDirOption is retained for existing manifests; new routes can use
	// DirectoryOption for any harness-specific skill directory.
	PluginDirOption string   `yaml:"plugin_dir_option,omitempty"`
	DirectoryOption string   `yaml:"directory_option,omitempty"`
	PersonalDirs    []string `yaml:"personal_dirs,omitempty"`
}

func (skills Skills) SkillDirOption() string {
	if skills.DirectoryOption != "" {
		return skills.DirectoryOption
	}
	return skills.PluginDirOption
}

type ConfigFile struct {
	Paths []string `yaml:"paths"`
	// Create allows TauShell to create an empty file when none exists.
	Create      bool   `yaml:"create,omitempty"`
	MissingHint string `yaml:"missing_hint,omitempty"`
}

type Install struct {
	VendorURL  string `yaml:"vendor_url"`
	WingetID   string `yaml:"winget_id,omitempty"`
	NpmPackage string `yaml:"npm_package,omitempty"`
}

type Brand struct {
	// Mark is a bundled vendor mark ("claude", "copilot") or "monogram".
	Mark string `yaml:"mark" json:"mark"`
	// Text is the monogram letters (at most 2); it defaults to the first
	// letters of short.
	Text  string `yaml:"text,omitempty" json:"text,omitempty"`
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
}
