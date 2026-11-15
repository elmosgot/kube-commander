package azure

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elmosgot/kube-commander/internal/config"
)

func TestEnsureCredentialsRequiresAzure(test *testing.T) {
	err := EnsureCredentials(config.Environment{Context: "x"})
	if err == nil || !strings.Contains(err.Error(), "no azure block") {
		test.Fatalf("got %v", err)
	}
}

func TestEnsureCredentialsRequiresFields(test *testing.T) {
	err := EnsureCredentials(config.Environment{
		Context: "x",
		Azure:   &config.AzureConfig{Subscription: "sub"},
	})
	if err == nil || !strings.Contains(err.Error(), "resource_group") {
		test.Fatalf("got %v", err)
	}
}

func TestGetCredentialsArgsAlwaysIncludesFile(test *testing.T) {
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
		test.Fatalf("missing --file %q in %v", kubeconfig, args)
	}
	if !filepath.IsAbs(kubeconfig) && !strings.Contains(kubeconfig, ".kube") {
		test.Fatalf("unexpected kubeconfig path %q", kubeconfig)
	}
	if !strings.Contains(joined, "--subscription 00000000-0000-0000-0000-000000000002") {
		test.Fatalf("missing subscription in %v", args)
	}
	if !strings.Contains(joined, "--overwrite-existing") {
		test.Fatalf("missing overwrite in %v", args)
	}
}

func TestGetCredentialsArgsExplicitKubeconfig(test *testing.T) {
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
		test.Fatalf("args = %v", args)
	}
}

func TestResolveAzBinaryPrefersUsrBin(test *testing.T) {
	bin := ResolveAzBinary()
	if bin != "/usr/bin/az" && bin != "az" {
		test.Fatalf("unexpected az binary %q", bin)
	}
}

func TestLoginArgsWithSupport(test *testing.T) {
	args := LoginArgs("00000000-0000-0000-0000-000000000002", true)
	joined := strings.Join(args, " ")
	if joined != "login -s 00000000-0000-0000-0000-000000000002 -o none" {
		test.Fatalf("LoginArgs = %v", args)
	}
}

func TestLoginArgsWithoutSupport(test *testing.T) {
	args := LoginArgs("00000000-0000-0000-0000-000000000002", false)
	joined := strings.Join(args, " ")
	if joined != "login -o none" {
		test.Fatalf("LoginArgs = %v", args)
	}
}

func TestPrepareLoginCmdSuppressesPicker(test *testing.T) {
	sub := "00000000-0000-0000-0000-000000000002"
	cmd := PrepareLoginCmd(sub)
	if cmd == nil {
		test.Fatal("expected command")
	}

	foundPicker := false
	foundOutput := false
	for _, e := range cmd.Env {
		if e == EnvLoginExperienceV2+"=off" {
			foundPicker = true
		}
		if e == EnvCoreOutput+"=none" {
			foundOutput = true
		}
	}
	if !foundPicker {
		test.Fatalf("missing %s=off in env %v", EnvLoginExperienceV2, cmd.Env)
	}
	if !foundOutput {
		test.Fatalf("missing %s=none in env %v", EnvCoreOutput, cmd.Env)
	}
	if cmd.Stdout == nil {
		test.Fatal("expected Stdout redirected away from TTY")
	}

	supports := DetectLoginSubscriptionSupport()
	wantArgs := LoginArgs(sub, supports)
	gotArgs := cmd.Args[1:] // skip binary
	if strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		test.Fatalf("args = %v want %v", gotArgs, wantArgs)
	}
}

func TestAccountSetArgs(test *testing.T) {
	args := AccountSetArgs("00000000-0000-0000-0000-000000000002")
	joined := strings.Join(args, " ")
	if joined != "account set --subscription 00000000-0000-0000-0000-000000000002" {
		test.Fatalf("AccountSetArgs = %v", args)
	}
}

func TestSupportsLoginSubscription(test *testing.T) {
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
			test.Fatalf("%d.%d.%d: got %v want %v", tc.maj, tc.min, tc.pat, got, tc.want)
		}
	}
}

func TestParseAzVersionJSON(test *testing.T) {
	maj, min, pat, err := ParseAzVersion(`{"azure-cli":"2.79.0","azure-cli-core":"2.79.0"}`)
	if err != nil {
		test.Fatal(err)
	}
	if maj != 2 || min != 79 || pat != 0 {
		test.Fatalf("got %d.%d.%d", maj, min, pat)
	}
}

func TestParseAzVersionText(test *testing.T) {
	const out = "azure-cli                         2.87.1\n" +
		"core                              2.87.1\n"
	maj, min, pat, err := ParseAzVersion(out)
	if err != nil {
		test.Fatal(err)
	}
	if maj != 2 || min != 87 || pat != 1 {
		test.Fatalf("got %d.%d.%d", maj, min, pat)
	}
}

func TestParseAzVersionEmpty(test *testing.T) {
	_, _, _, err := ParseAzVersion("")
	if err == nil {
		test.Fatal("expected error")
	}
}
