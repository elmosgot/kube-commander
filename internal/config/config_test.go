package config

import (
	"os"
	"path/filepath"
	"testing"
)

const envDevelop = "develop"

func TestLoadExample(test *testing.T) {
	path := filepath.Join("..", "..", "configs", "example.yaml")
	cfg, resolved, err := Load(path, "")
	if err != nil {
		test.Fatalf("Load: %v", err)
	}
	if resolved != path {
		test.Fatalf("resolved = %q, want %q", resolved, path)
	}
	if len(cfg.Pools) != 2 {
		test.Fatalf("pools = %d, want 2", len(cfg.Pools))
	}
	if cfg.Pools[0].Name != "pool-x" {
		test.Fatalf("first pool = %q, want pool-x", cfg.Pools[0].Name)
	}
	if len(cfg.Pools[0].Environments) != 4 {
		test.Fatalf("pool-x environments = %d, want 4", len(cfg.Pools[0].Environments))
	}
	env := cfg.Pools[0].Environments[0]
	if env.Context != "pool-x-dev" {
		test.Fatalf("context = %q", env.Context)
	}
	if env.Namespace != "app" {
		test.Fatalf("namespace = %q", env.Namespace)
	}
	if env.Azure == nil || env.Azure.Cluster != "aks-pool-x-dev" {
		test.Fatalf("azure = %+v", env.Azure)
	}
	if cfg.Pools[1].Name != "pool-y" {
		test.Fatalf("second pool = %q, want pool-y", cfg.Pools[1].Name)
	}
}

func TestValidateRejectsEmpty(test *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err == nil {
		test.Fatal("expected error for empty config")
	}
}

func TestValidateRejectsDuplicatePool(test *testing.T) {
	cfg := &Config{
		Pools: []Pool{
			{Name: "a", Environments: []Environment{{Name: envDevelop, Context: "a-dev"}}},
			{Name: "a", Environments: []Environment{{Name: envDevelop, Context: "b-dev"}}},
		},
	}
	if err := cfg.Validate(); err == nil {
		test.Fatal("expected duplicate pool error")
	}
}

func TestValidateRejectsIncompleteAzure(test *testing.T) {
	cfg := &Config{
		Pools: []Pool{
			{
				Name: "a",
				Environments: []Environment{{
					Name:    envDevelop,
					Context: "a-dev",
					Azure:   &AzureConfig{Subscription: "sub-only"},
				}},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		test.Fatal("expected azure validation error")
	}
}

func TestLoadFallsBackToExample(test *testing.T) {
	dir := test.TempDir()
	example := filepath.Join(dir, "example.yaml")
	content := []byte("pools:\n  - name: demo\n    environments:\n      - name: develop\n        context: demo-dev\n")
	if err := os.WriteFile(example, content, 0o600); err != nil {
		test.Fatal(err)
	}

	missing := filepath.Join(dir, "missing.yaml")
	_, _, err := Load(missing, example)
	if err == nil {
		test.Fatal("expected error when explicit path is missing")
	}
}

func TestExpandHome(test *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		test.Fatal(err)
	}
	got := expandHome("~/kube/config")
	want := filepath.Join(home, "kube/config")
	if got != want {
		test.Fatalf("expandHome = %q, want %q", got, want)
	}
}

func TestResolveKubeconfigDefault(test *testing.T) {
	got := ResolveKubeconfig(Environment{Context: "x"})
	want := DefaultKubeconfigPath()
	if got != want {
		test.Fatalf("ResolveKubeconfig = %q, want %q", got, want)
	}
}

func TestResolveKubeconfigExplicit(test *testing.T) {
	got := ResolveKubeconfig(Environment{Context: "x", Kubeconfig: "/tmp/custom.kubeconfig"})
	if got != "/tmp/custom.kubeconfig" {
		test.Fatalf("ResolveKubeconfig = %q", got)
	}
}
