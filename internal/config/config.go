package config

import (
	"fmt"
	"os"
	"strings"
	"time"

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
	IdleTimeout time.Duration
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
		runtimeRule, err := validateRule(rule)

		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}

		if _, exists := names[runtimeRule.Name]; exists {
			return nil, fmt.Errorf("rule %d: duplicate rule name %q", i+1, runtimeRule.Name)
		}

		if _, exists := listeners[runtimeRule.Listen]; exists {
			return nil, fmt.Errorf("rule %q: duplicate listen address %q", runtimeRule.Name, runtimeRule.Listen)
		}

		names[runtimeRule.Name] = struct{}{}
		listeners[runtimeRule.Listen] = struct{}{}
		rules = append(rules, runtimeRule)
	}

	return rules, nil
}

func validateRule(rule Rule) (RuntimeRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Listen = strings.TrimSpace(rule.Listen)
	rule.Target = strings.TrimSpace(rule.Target)
	rule.IdleTimeout = strings.TrimSpace(rule.IdleTimeout)

	if rule.Name == "" {
		return RuntimeRule{}, fmt.Errorf("name is required")
	}

	if rule.Listen == "" {
		return RuntimeRule{}, fmt.Errorf("listen address is required")
	}

	if rule.Target == "" {
		return RuntimeRule{}, fmt.Errorf("target address is required")
	}

	// if listen and target addresses are the same raise error
	if rule.Listen == rule.Target {
		return RuntimeRule{}, fmt.Errorf("listen and target addresses cannot be identical")
	}

	idleTimeout := 5 * time.Minute

	// if timeout in config is not empty, parse given timeout into time.duration
	if rule.IdleTimeout != "" {
		parsedTimeout, err := time.ParseDuration(rule.IdleTimeout)

		if err != nil {
			return RuntimeRule{}, fmt.Errorf("invalid idle timeout %q: %w", rule.IdleTimeout, err)
		}

		if parsedTimeout < 0 {
			return RuntimeRule{}, fmt.Errorf("idle timeout cannot be negative")
		}

		idleTimeout = parsedTimeout
	}

	return RuntimeRule{
		Name:        rule.Name,
		Listen:      rule.Listen,
		Target:      rule.Target,
		IdleTimeout: idleTimeout,
	}, nil
}
