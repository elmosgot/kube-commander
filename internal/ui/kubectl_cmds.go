package ui

import (
	"fmt"
	"strings"

	"github.com/elmosgot/kube-commander/internal/config"
)

// kubectlLineForView returns the primary kubectl-equivalent line for the current right view.
func (m Model) kubectlLineForView() string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "# select an environment"
	}
	base := kubectlBase(env)
	switch m.rightView {
	case viewWorkloads:
		ns := m.namespace
		if ns == "" {
			ns = env.Namespace
		}
		if ns == "" {
			return base + " get deployments -A"
		}
		return fmt.Sprintf("%s -n %s get deployments", base, shellQuote(ns))
	case viewPods:
		return fmt.Sprintf("%s -n %s get pods -l '<selector from deploy/%s>'",
			base, shellQuote(m.namespace), m.deployment)
	case viewPodDetail:
		return fmt.Sprintf("%s -n %s get pod %s -o wide", base, shellQuote(m.namespace), shellQuote(m.podName))
	case viewPodLogs:
		return m.kubectlLogsFollowLine()
	default:
		return base + " get namespaces"
	}
}

func (m Model) kubectlLogsFollowLine() string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "kubectl logs --follow"
	}
	base := kubectlBase(env)
	ctr := m.selectedContainerName()
	line := fmt.Sprintf("%s -n %s logs %s --tail=200 --follow", base, shellQuote(m.namespace), shellQuote(m.podName))
	if ctr != "" {
		line += " -c " + shellQuote(ctr)
	}
	return line
}

func (m Model) kubectlLineNamespaces() string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "kubectl get namespaces"
	}
	return kubectlBase(env) + " get namespaces"
}

func (m Model) kubectlLineWorkloads(namespace string) string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "kubectl get deployments"
	}
	base := kubectlBase(env)
	if namespace == "" {
		return base + " get deployments -A"
	}
	return fmt.Sprintf("%s -n %s get deployments", base, shellQuote(namespace))
}

func (m Model) kubectlLinePods(namespace, deployment string) string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "kubectl get pods"
	}
	return fmt.Sprintf("%s -n %s get pods -l '<selector from deploy/%s>'",
		kubectlBase(env), shellQuote(namespace), deployment)
}

func (m Model) kubectlLinePodDetail(namespace, pod string) string {
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return "kubectl get pod"
	}
	return fmt.Sprintf("%s -n %s get pod %s -o wide",
		kubectlBase(env), shellQuote(namespace), shellQuote(pod))
}

func kubectlExecLine(env config.Environment, namespace, pod, container string) string {
	args := kubectlExecArgs(env, namespace, pod, container)
	return "kubectl " + strings.Join(quoteArgs(args), " ")
}

func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = shellQuote(a)
	}
	return out
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
