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
	Expose []int `yaml:"expose"`
}
