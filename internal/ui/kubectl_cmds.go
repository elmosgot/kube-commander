package ui

import (
	"fmt"
	"strings"

	"github.com/elmosgot/kube-commander/internal/config"
)

func (model Model) kubectlLogsFollowLine() string {
	env, _, _, ok := model.selectedEnv()
	if !ok {
		return "kubectl logs --follow"
	}
	base := kubectlBase(env)
	ctr := model.selectedContainerName()
	line := fmt.Sprintf(
		"%s -n %s logs %s --tail=200 --follow",
		base, shellQuote(model.namespace), shellQuote(model.podName),
	)
	if ctr != "" {
		line += " -c " + shellQuote(ctr)
	}
	return line
}

func (model Model) kubectlLineNamespaces() string {
	env, _, _, ok := model.selectedEnv()
	if !ok {
		return "kubectl get namespaces"
	}
	return kubectlBase(env) + " get namespaces"
}

func (model Model) kubectlLineWorkloads(namespace string) string {
	env, _, _, ok := model.selectedEnv()
	if !ok {
		return "kubectl get deployments"
	}
	base := kubectlBase(env)
	if namespace == "" {
		return base + " get deployments -A"
	}
	return fmt.Sprintf("%s -n %s get deployments", base, shellQuote(namespace))
}

func (model Model) kubectlLinePods(namespace, deployment string) string {
	env, _, _, ok := model.selectedEnv()
	if !ok {
		return "kubectl get pods"
	}
	return fmt.Sprintf("%s -n %s get pods -l '<selector from deploy/%s>'",
		kubectlBase(env), shellQuote(namespace), deployment)
}

func (model Model) kubectlLinePodDetail(namespace, pod string) string {
	env, _, _, ok := model.selectedEnv()
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

func shellQuote(input string) string {
	if input == "" {
		return input
	}
	if strings.ContainsAny(input, " \t\"'\\$`") {
		return "'" + strings.ReplaceAll(input, "'", `'\''`) + "'"
	}
	return input
}
