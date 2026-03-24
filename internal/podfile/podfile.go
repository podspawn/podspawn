package podfile

type Podfile struct {
	Base          string            `yaml:"base"`
	Packages      []string          `yaml:"packages"`
	Shell         string            `yaml:"shell"`
	Dotfiles      *DotfilesConfig   `yaml:"dotfiles"`
	Repos         []RepoConfig      `yaml:"repos"`
	Env           map[string]string `yaml:"env"`
	Services      []ServiceConfig   `yaml:"services"`
	Ports         PortsConfig       `yaml:"ports"`
	Resources     ResourcesConfig   `yaml:"resources"`
	OnCreate      string            `yaml:"on_create"`
	OnStart       string            `yaml:"on_start"`
	ExtraCommands []string          `yaml:"extra_commands"`
	Extends       string            `yaml:"extends"`
	Name          string            `yaml:"name"`
	Mount         string            `yaml:"mount"`
	Mode          string            `yaml:"mode"`
	Workspace     string            `yaml:"workspace"`
}

type ServiceConfig struct {
	Name    string            `yaml:"name"`
	Image   string            `yaml:"image"`
	Ports   []int             `yaml:"ports"`
	Env     map[string]string `yaml:"env"`
	Volumes []string          `yaml:"volumes"`
}

type DotfilesConfig struct {
	Repo    string `yaml:"repo"`
	Install string `yaml:"install"`
}

type RepoConfig struct {
	URL    string `yaml:"url"`
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
}

type ResourcesConfig struct {
	CPUs   float64 `yaml:"cpus"`
	Memory string  `yaml:"memory"`
}

type PortsConfig struct {
	Expose   []int  `yaml:"expose"`
	Strategy string `yaml:"strategy"`
}

// MergeFlags tracks which fields use bang-replace syntax (e.g. packages!:).
// Only relevant during extends resolution; discarded afterward.
type MergeFlags struct {
	PackagesReplace      bool
	EnvReplace           bool
	ServicesReplace      bool
	ReposReplace         bool
	ExtraCommandsReplace bool
	OnCreateReplace      bool
	OnStartReplace       bool
}

// RawPodfile pairs a parsed Podfile with its merge flags from bang syntax.
type RawPodfile struct {
	Podfile
	Flags MergeFlags
}
