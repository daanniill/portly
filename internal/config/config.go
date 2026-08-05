package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

type File struct {
	Rules []Rule `yaml:"rules"`
}

type Rule struct {
	Name        string `yaml:"name"`
	Listen      string `yaml:"listen"`
	Target      string `yaml:"target"`
	IdleTimeout string `yaml:"idle_timeout"`
}

type RuntimeRule struct {
	Name        string
	Listen      string
	Target      string
	IdleTimeout string
}

func Load(path string) ([]RuntimeRule, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var configFile File
	
	if err := yaml.Unmarshal(data, &configFile); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if len(configFile.Rules) == 0 {
		return nil, fmt.Errorf("config contains no forwarding rules")
	}

	rules := make([]RuntimeRule, 0, len(configFile.Rules))

	// sets of names and listen addresses, aka empty structs
	names := make(map[string]struct{})
	listeners := make(map[string]struct{})
	
	for i, rule := range configFile.Rules {
		// validate rules later

		if _, exists := names[rule.Name]; exists {
			return nil, fmt.Errorf("rule %d: duplicate rule name %q", i+1, rule.Name)
		}

		if _, exists := listeners[rule.Listen]; exists {
			return nil, fmt.Errorf("rule %q: duplicate listen address %q", rule.Name, rule.Listen)
		}

		names[rule.Name] = struct{}{}
		listeners[rule.Listen] = struct{}{}
		rules = append(rules, rule)
	}

	return rules, nil
}