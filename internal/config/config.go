package config

type File struct {
	Rules []Rule `yaml:"rules"`
}

type Rule struct {
	Name        string `yaml:"name"`
	Listen      string `yaml:"listen"`
	Target      string `yaml:"target"`
	IdleTimeout string `yaml:"idle_timeout"`
}

type ParsedRule struct {
	Name        string
	Listen      string
	Target      string
	IdleTimeout string
}
