package azure

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elwin/kube-commander/internal/config"
)

// EnsureCredentials runs `az aks get-credentials` into the resolved home kubeconfig,
// then converts that file with kubelogin for Azure CLI auth.
// az login remains a manual prerequisite.
func EnsureCredentials(env config.Environment) error {
	if env.Azure == nil {
		return fmt.Errorf("no azure block configured for context %q", env.Context)
	}
	if env.Azure.ResourceGroup == "" || env.Azure.Cluster == "" {
		return fmt.Errorf("azure.resource_group and azure.cluster are required")
	}

	kubeconfig := config.ResolveKubeconfig(env)
	args := getCredentialsArgs(env, kubeconfig)

	azBin := ResolveAzBinary()
	if err := runCmd(azBin, args...); err != nil {
		return fmt.Errorf("az aks get-credentials: %w", err)
	}

	kubeloginBin := resolveKubeloginBinary()
	if err := runCmd(kubeloginBin, "convert-kubeconfig", "-l", "azurecli", "--kubeconfig", kubeconfig); err != nil {
		return fmt.Errorf("kubelogin convert-kubeconfig: %w", err)
	}
	return nil
}

// getCredentialsArgs builds az aks get-credentials arguments. Always includes --file.
func getCredentialsArgs(env config.Environment, kubeconfig string) []string {
	args := []string{
		"aks", "get-credentials",
		"--resource-group", env.Azure.ResourceGroup,
		"--name", env.Azure.Cluster,
		"--context", env.Context,
		"--overwrite-existing",
		"--file", kubeconfig,
	}
	if env.Azure.Subscription != "" {
		args = append(args, "--subscription", env.Azure.Subscription)
	}
	return args
}

// LoginArgs returns arguments for interactive `az login -s {subscription}`.
func LoginArgs(subscription string) []string {
	return []string{"login", "-s", subscription}
}

// ResolveAzBinary prefers /usr/bin/az to avoid Windows az on a polluted WSL PATH.
func ResolveAzBinary() string {
	const preferred = "/usr/bin/az"
	if isExecutable(preferred) {
		return preferred
	}
	return "az"
}

func resolveKubeloginBinary() string {
	const preferred = "/usr/local/bin/kubelogin"
	if isExecutable(preferred) {
		return preferred
	}
	return "kubelogin"
}

func isExecutable(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Mode()&0o111 != 0
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
