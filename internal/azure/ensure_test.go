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

func TestLoginArgsWithSupport(t *testing.T) {
	args := LoginArgs("00000000-0000-0000-0000-000000000002", true)
	joined := strings.Join(args, " ")
	if joined != "login -s 00000000-0000-0000-0000-000000000002" {
		t.Fatalf("LoginArgs = %v", args)
	}
}

func TestLoginArgsWithoutSupport(t *testing.T) {
	args := LoginArgs("00000000-0000-0000-0000-000000000002", false)
	joined := strings.Join(args, " ")
	if joined != "login" {
		t.Fatalf("LoginArgs = %v", args)
	}
}

func TestPrepareLoginCmdSuppressesPicker(t *testing.T) {
	sub := "00000000-0000-0000-0000-000000000002"
	cmd := PrepareLoginCmd(sub)
	if cmd == nil {
		t.Fatal("expected command")
	}

	found := false
	for _, e := range cmd.Env {
		if e == EnvLoginExperienceV2+"=off" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing %s=off in env %v", EnvLoginExperienceV2, cmd.Env)
	}

	supports := DetectLoginSubscriptionSupport()
	wantArgs := LoginArgs(sub, supports)
	gotArgs := cmd.Args[1:] // skip binary
	if strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("args = %v want %v", gotArgs, wantArgs)
	}
}

func TestAccountSetArgs(t *testing.T) {
	args := AccountSetArgs("00000000-0000-0000-0000-000000000002")
	joined := strings.Join(args, " ")
	if joined != "account set --subscription 00000000-0000-0000-0000-000000000002" {
		t.Fatalf("AccountSetArgs = %v", args)
	}
}

func TestSupportsLoginSubscription(t *testing.T) {
	cases := []struct {
		maj, min, pat int
		want          bool
	}{
		{2, 86, 0, false},
		{2, 86, 99, false},
		{2, 87, 0, true},
		{2, 87, 1, true},
		{2, 88, 0, true},
		{3, 0, 0, true},
		{1, 99, 99, false},
	}
	for _, tc := range cases {
		got := SupportsLoginSubscription(tc.maj, tc.min, tc.pat)
		if got != tc.want {
			t.Fatalf("%d.%d.%d: got %v want %v", tc.maj, tc.min, tc.pat, got, tc.want)
		}
	}
}

func TestParseAzVersionJSON(t *testing.T) {
	maj, min, pat, err := ParseAzVersion(`{"azure-cli":"2.79.0","azure-cli-core":"2.79.0"}`)
	if err != nil {
		t.Fatal(err)
	}
	if maj != 2 || min != 79 || pat != 0 {
		t.Fatalf("got %d.%d.%d", maj, min, pat)
	}
}

func TestParseAzVersionText(t *testing.T) {
	maj, min, pat, err := ParseAzVersion("azure-cli                         2.87.1\ncore                              2.87.1\n")
	if err != nil {
		t.Fatal(err)
	}
	if maj != 2 || min != 87 || pat != 1 {
		t.Fatalf("got %d.%d.%d", maj, min, pat)
	}
}

func TestParseAzVersionEmpty(t *testing.T) {
	_, _, _, err := ParseAzVersion("")
	if err == nil {
		t.Fatal("expected error")
	}
}
