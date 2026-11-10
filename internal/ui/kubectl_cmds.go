package ui

import (
	"fmt"
	"strings"

	"github.com/elwin/kube-commander/internal/config"
)

// kubectlRecipes returns equivalent kubectl lines for the current right-pane view.
// These are not executed; client-go performs the real fetches.
func (m Model) kubectlRecipes() []string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return []string{"# select an environment"}
	}
	base := kubectlBase(env)
	switch m.rightView {
	case viewWorkloads:
		ns := m.namespace
		if ns == "" {
			ns = env.Namespace
		}
		if ns == "" {
			return []string{base + " get deployments -A"}
		}
		return []string{fmt.Sprintf("%s -n %s get deployments", base, shellQuote(ns))}
	case viewPods:
		ns := m.namespace
		dep := m.deployment
		return []string{
			fmt.Sprintf("%s -n %s get deployment %s", base, shellQuote(ns), shellQuote(dep)),
			fmt.Sprintf("%s -n %s get pods -l '<selector from deploy/%s>'", base, shellQuote(ns), dep),
		}
	case viewPodDetail:
		return []string{
			fmt.Sprintf("%s -n %s get pod %s -o wide", base, shellQuote(m.namespace), shellQuote(m.podName)),
		}
	case viewPodLogs:
		ctr := m.selectedContainerName()
		line := fmt.Sprintf("%s -n %s logs %s --tail=200", base, shellQuote(m.namespace), shellQuote(m.podName))
		if ctr != "" {
			line += " -c " + shellQuote(ctr)
		}
		return []string{line}
	default:
		// Namespaces view also aggregates cluster-wide deploy/pod counts in client-go.
		return []string{
			base + " get namespaces",
			base + " get deployments -A",
			base + " get pods -A",
		}
	}
}

func kubectlBase(env config.Environment) string {
	parts := []string{"kubectl", "--context", shellQuote(env.Context)}
	kc := config.ResolveKubeconfig(env)
	if kc != "" {
		parts = append(parts, "--kubeconfig", shellQuote(kc))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " \t\"'\\$`") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
