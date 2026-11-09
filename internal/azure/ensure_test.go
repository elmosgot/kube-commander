package azure

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elwin/kube-commander/internal/config"
)

func TestEnsureCredentialsRequiresAzure(t *testing.T) {
	err := EnsureCredentials(config.Environment{Context: "x"})
	if err == nil || !strings.Contains(err.Error(), "no azure block") {
		t.Fatalf("got %v", err)
	}
}

func TestEnsureCredentialsRequiresFields(t *testing.T) {
	err := EnsureCredentials(config.Environment{
		Context: "x",
		Azure:   &config.AzureConfig{Subscription: "sub"},
	})
	if err == nil || !strings.Contains(err.Error(), "resource_group") {
		t.Fatalf("got %v", err)
	}
}

func TestGetCredentialsArgsAlwaysIncludesFile(t *testing.T) {
	env := config.Environment{
		Context: "pool-x-test",
		Azure: &config.AzureConfig{
			Subscription:  "00000000-0000-0000-0000-000000000002",
			ResourceGroup: "rg-pool-x-test",
			Cluster:       "aks-pool-x-test",
		},
	}
	kubeconfig := config.ResolveKubeconfig(env)
	args := getCredentialsArgs(env, kubeconfig)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--file "+kubeconfig) {
		t.Fatalf("missing --file %q in %v", kubeconfig, args)
	}
	if !filepath.IsAbs(kubeconfig) && !strings.Contains(kubeconfig, ".kube") {
		t.Fatalf("unexpected kubeconfig path %q", kubeconfig)
	}
	if !strings.Contains(joined, "--subscription 00000000-0000-0000-0000-000000000002") {
		t.Fatalf("missing subscription in %v", args)
	}
	if !strings.Contains(joined, "--overwrite-existing") {
		t.Fatalf("missing overwrite in %v", args)
	}
}

func TestGetCredentialsArgsExplicitKubeconfig(t *testing.T) {
	env := config.Environment{
		Context:    "ctx",
		Kubeconfig: "/tmp/custom.kubeconfig",
		Azure: &config.AzureConfig{
			ResourceGroup: "rg",
			Cluster:       "cluster",
		},
	}
	path := config.ResolveKubeconfig(env)
	args := getCredentialsArgs(env, path)
	if !strings.Contains(strings.Join(args, " "), "--file /tmp/custom.kubeconfig") {
		t.Fatalf("args = %v", args)
	}
}

func TestResolveAzBinaryPrefersUsrBin(t *testing.T) {
	bin := ResolveAzBinary()
	if bin != "/usr/bin/az" && bin != "az" {
		t.Fatalf("unexpected az binary %q", bin)
	}
}

func TestLoginArgs(t *testing.T) {
	args := LoginArgs("00000000-0000-0000-0000-000000000002")
	joined := strings.Join(args, " ")
	if joined != "login -s 00000000-0000-0000-0000-000000000002" {
		t.Fatalf("LoginArgs = %v", args)
	}
}
