package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AzureConfig holds AKS get-credentials parameters for an environment.
type AzureConfig struct {
	Subscription  string `yaml:"subscription,omitempty"`
	ResourceGroup string `yaml:"resource_group"`
	Cluster       string `yaml:"cluster"`
}

// Environment maps a logical DTAP name to a kube context.
type Environment struct {
	Name       string       `yaml:"name"`
	Context    string       `yaml:"context"`
	Namespace  string       `yaml:"namespace,omitempty"`
	Kubeconfig string       `yaml:"kubeconfig,omitempty"`
	Azure      *AzureConfig `yaml:"azure,omitempty"`
}

// Pool groups related environments under one product or team name.
type Pool struct {
	Name         string        `yaml:"name"`
	Environments []Environment `yaml:"environments"`
}

// Config is the on-disk kube-commander configuration.
type Config struct {
	Pools []Pool `yaml:"pools"`
}

// DefaultPath returns ~/.config/kube-commander/config.yaml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "kube-commander", "config.yaml")
}

// DefaultKubeconfigPath returns $HOME/.kube/config.
func DefaultKubeconfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".kube", "config")
	}
	return filepath.Join(home, ".kube", "config")
}

// ResolveKubeconfig returns the kubeconfig file for an environment.
// Explicit env.Kubeconfig wins; otherwise $HOME/.kube/config is used.
func ResolveKubeconfig(env Environment) string {
	if strings.TrimSpace(env.Kubeconfig) != "" {
		return env.Kubeconfig
	}
	return DefaultKubeconfigPath()
}

// Load reads and validates a config file. If path is empty, DefaultPath is used.
// When the default path is missing, examplePath is loaded instead when provided.
func Load(path, examplePath string) (*Config, string, error) {
	if path == "" {
		path = DefaultPath()
	}

	resolved := path
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && path == DefaultPath() && examplePath != "" {
			data, err = os.ReadFile(examplePath)
			if err != nil {
				return nil, "", fmt.Errorf("default config %q missing and example %q unreadable: %w", path, examplePath, err)
			}
			resolved = examplePath
		} else {
			return nil, "", fmt.Errorf("read config %q: %w", path, err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("parse config %q: %w", resolved, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, "", err
	}
	cfg.expandPaths()
	return &cfg, resolved, nil
}

// Validate checks required fields and uniqueness.
func (c *Config) Validate() error {
	if len(c.Pools) == 0 {
		return fmt.Errorf("config must define at least one pool")
	}

	poolNames := make(map[string]struct{}, len(c.Pools))
	for i, pool := range c.Pools {
		name := strings.TrimSpace(pool.Name)
		if name == "" {
			return fmt.Errorf("pools[%d]: name is required", i)
		}
		if _, exists := poolNames[name]; exists {
			return fmt.Errorf("duplicate pool name %q", name)
		}
		poolNames[name] = struct{}{}
		c.Pools[i].Name = name

		if len(pool.Environments) == 0 {
			return fmt.Errorf("pool %q: at least one environment is required", name)
		}

		envNames := make(map[string]struct{}, len(pool.Environments))
		for j, env := range pool.Environments {
			envName := strings.TrimSpace(env.Name)
			if envName == "" {
				return fmt.Errorf("pool %q environments[%d]: name is required", name, j)
			}
			if _, exists := envNames[envName]; exists {
				return fmt.Errorf("pool %q: duplicate environment name %q", name, envName)
			}
			envNames[envName] = struct{}{}

			ctx := strings.TrimSpace(env.Context)
			if ctx == "" {
				return fmt.Errorf("pool %q environment %q: context is required", name, envName)
			}

			ns := strings.TrimSpace(env.Namespace)
			kc := strings.TrimSpace(env.Kubeconfig)

			if env.Azure != nil {
				rg := strings.TrimSpace(env.Azure.ResourceGroup)
				cluster := strings.TrimSpace(env.Azure.Cluster)
				sub := strings.TrimSpace(env.Azure.Subscription)
				if rg == "" {
					return fmt.Errorf("pool %q environment %q: azure.resource_group is required", name, envName)
				}
				if cluster == "" {
					return fmt.Errorf("pool %q environment %q: azure.cluster is required", name, envName)
				}
				c.Pools[i].Environments[j].Azure = &AzureConfig{
					Subscription:  sub,
					ResourceGroup: rg,
					Cluster:       cluster,
				}
			}

			c.Pools[i].Environments[j].Name = envName
			c.Pools[i].Environments[j].Context = ctx
			c.Pools[i].Environments[j].Namespace = ns
			c.Pools[i].Environments[j].Kubeconfig = kc
		}
	}
	return nil
}

func (c *Config) expandPaths() {
	for i := range c.Pools {
		for j := range c.Pools[i].Environments {
			kc := c.Pools[i].Environments[j].Kubeconfig
			if kc == "" {
				continue
			}
			c.Pools[i].Environments[j].Kubeconfig = expandHome(kc)
		}
	}
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
