package config

import (
	"os"
	"path/filepath"
	"testing"
)

const envDevelop = "develop"

func TestLoadExample(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "example.yaml")
	cfg, resolved, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
	if len(cfg.Pools) != 2 {
		t.Fatalf("pools = %d, want 2", len(cfg.Pools))
	}
	if cfg.Pools[0].Name != "pool-x" {
		t.Fatalf("first pool = %q, want pool-x", cfg.Pools[0].Name)
	}
	if len(cfg.Pools[0].Environments) != 4 {
		t.Fatalf("pool-x environments = %d, want 4", len(cfg.Pools[0].Environments))
	}
	env := cfg.Pools[0].Environments[0]
	if env.Context != "pool-x-dev" {
		t.Fatalf("context = %q", env.Context)
	}
	if env.Namespace != "app" {
		t.Fatalf("namespace = %q", env.Namespace)
	}
	if env.Azure == nil || env.Azure.Cluster != "aks-pool-x-dev" {
		t.Fatalf("azure = %+v", env.Azure)
	}
	if cfg.Pools[1].Name != "pool-y" {
		t.Fatalf("second pool = %q, want pool-y", cfg.Pools[1].Name)
	}
}

func TestValidateRejectsEmpty(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestValidateRejectsDuplicatePool(t *testing.T) {
	cfg := &Config{
		Pools: []Pool{
			{Name: "a", Environments: []Environment{{Name: envDevelop, Context: "a-dev"}}},
			{Name: "a", Environments: []Environment{{Name: envDevelop, Context: "b-dev"}}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate pool error")
	}
}

func TestValidateRejectsIncompleteAzure(t *testing.T) {
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
		t.Fatal("expected azure validation error")
	}
}

func TestLoadFallsBackToExample(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "example.yaml")
	content := []byte("pools:\n  - name: demo\n    environments:\n      - name: develop\n        context: demo-dev\n")
	if err := os.WriteFile(example, content, 0o600); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(dir, "missing.yaml")
	_, _, err := Load(missing, example)
	if err == nil {
		t.Fatal("expected error when explicit path is missing")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := expandHome("~/kube/config")
	want := filepath.Join(home, "kube/config")
	if got != want {
		t.Fatalf("expandHome = %q, want %q", got, want)
	}
}

func TestResolveKubeconfigDefault(t *testing.T) {
	got := ResolveKubeconfig(Environment{Context: "x"})
	want := DefaultKubeconfigPath()
	if got != want {
		t.Fatalf("ResolveKubeconfig = %q, want %q", got, want)
	}
}

func TestResolveKubeconfigExplicit(t *testing.T) {
	got := ResolveKubeconfig(Environment{Context: "x", Kubeconfig: "/tmp/custom.kubeconfig"})
	if got != "/tmp/custom.kubeconfig" {
		t.Fatalf("ResolveKubeconfig = %q", got)
	}
}
