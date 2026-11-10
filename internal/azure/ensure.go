package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/elwin/kube-commander/internal/applog"
	"github.com/elwin/kube-commander/internal/config"
)

// Minimum Azure CLI version that supports `az login --subscription` / `-s`.
const minLoginSubscriptionMajor, minLoginSubscriptionMinor, minLoginSubscriptionPatch = 2, 87, 0

// EnvLoginExperienceV2 is the Azure CLI env override for core.login_experience_v2.
// Setting it to "off" skips the interactive subscription picker for that process only.
const EnvLoginExperienceV2 = "AZURE_CORE_LOGIN_EXPERIENCE_V2"

// EnvCoreOutput is the Azure CLI env override for core.output.
const EnvCoreOutput = "AZURE_CORE_OUTPUT"

var (
	azVersionOnce sync.Once
	azVersionMaj  int
	azVersionMin  int
	azVersionPat  int
	azVersionErr  error
)

var semverRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

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

// LoginArgs returns arguments for interactive az login.
// When supportsLoginSub is true (Azure CLI >= 2.87.0), includes -s {subscription}.
// Always includes -o none so account JSON is not printed to the TUI.
func LoginArgs(subscription string, supportsLoginSub bool) []string {
	args := []string{"login"}
	if supportsLoginSub && subscription != "" {
		args = append(args, "-s", subscription)
	}
	return append(args, "-o", "none")
}

// PrepareLoginCmd builds an interactive az login command that suppresses the
// subscription picker and keeps stdout out of the TUI (session log + -o none).
// Stderr is left unset so Bubble Tea can wire device-code / error messages to the terminal.
func PrepareLoginCmd(subscription string) *exec.Cmd {
	supportsLoginSub := DetectLoginSubscriptionSupport()
	args := LoginArgs(subscription, supportsLoginSub)
	azBin := ResolveAzBinary()
	applog.Infof("starting interactive: %s %s (AZURE_CORE_LOGIN_EXPERIENCE_V2=off AZURE_CORE_OUTPUT=none)", azBin, strings.Join(args, " "))
	cmd := exec.Command(azBin, args...)
	cmd.Env = append(os.Environ(),
		EnvLoginExperienceV2+"=off",
		EnvCoreOutput+"=none",
	)
	cmd.Stdout = applog.Writer()
	return cmd
}

// AccountSetArgs returns arguments for `az account set --subscription`.
func AccountSetArgs(subscription string) []string {
	return []string{"account", "set", "--subscription", subscription}
}

// SetSubscription pins the active Azure subscription via `az account set`.
func SetSubscription(subscription string) error {
	if subscription == "" {
		return fmt.Errorf("subscription is required")
	}
	azBin := ResolveAzBinary()
	if err := runCmd(azBin, AccountSetArgs(subscription)...); err != nil {
		return fmt.Errorf("az account set: %w", err)
	}
	return nil
}

// SupportsLoginSubscription reports whether az login accepts --subscription / -s.
func SupportsLoginSubscription(major, minor, patch int) bool {
	if major != minLoginSubscriptionMajor {
		return major > minLoginSubscriptionMajor
	}
	if minor != minLoginSubscriptionMinor {
		return minor > minLoginSubscriptionMinor
	}
	return patch >= minLoginSubscriptionPatch
}

// ParseAzVersion extracts major.minor.patch from `az version -o json` or `az --version` text.
func ParseAzVersion(output string) (major, minor, patch int, err error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, 0, 0, fmt.Errorf("empty az version output")
	}

	var payload map[string]any
	if json.Unmarshal([]byte(output), &payload) == nil {
		if v, ok := payload["azure-cli"]; ok {
			if s, ok := v.(string); ok {
				return parseSemver(s)
			}
		}
	}

	return parseSemver(output)
}

func parseSemver(s string) (major, minor, patch int, err error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("no semver in %q", s)
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch, nil
}

// DetectLoginSubscriptionSupport runs az once per process to decide if login -s is available.
// On detection failure it returns false so callers use the portable login + account set path.
func DetectLoginSubscriptionSupport() bool {
	azVersionOnce.Do(func() {
		azBin := ResolveAzBinary()
		args := []string{"version", "-o", "json"}
		out, err := exec.Command(azBin, args...).Output()
		applog.Cmd(azBin, args, string(out), err)
		if err != nil {
			args = []string{"--version"}
			out, err = exec.Command(azBin, args...).Output()
			applog.Cmd(azBin, args, string(out), err)
		}
		if err != nil {
			azVersionErr = err
			return
		}
		azVersionMaj, azVersionMin, azVersionPat, azVersionErr = ParseAzVersion(string(out))
	})
	if azVersionErr != nil {
		return false
	}
	return SupportsLoginSubscription(azVersionMaj, azVersionMin, azVersionPat)
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
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	err := cmd.Run()
	output := buf.String()
	applog.Cmd(name, args, output, err)
	if err != nil {
		msg := strings.TrimSpace(output)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
