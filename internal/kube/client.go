package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/elmosgot/kube-commander/internal/config"
)

const (
	defaultTimeout = 8 * time.Second
	logsTimeout    = 15 * time.Second
	defaultTail    = 200
)

// NamespaceInfo is a summarized namespace row for the right pane.
type NamespaceInfo struct {
	Name            string
	DeploymentCount int
	ReadyPods       int
	TotalPods       int
}

// WorkloadInfo is a summarized deployment row for a namespace.
type WorkloadInfo struct {
	Name      string
	Ready     int32
	Desired   int32
	Available int32
	Updated   int32
	Age       string
}

// PodInfo is a summarized pod row under a deployment.
type PodInfo struct {
	Name       string
	Ready      string // e.g. 1/1
	Restarts   int32
	Status     string
	Node       string
	Age        string
	CPURequest string
	CPULimit   string
	MemRequest string
	MemLimit   string
}

// ContainerInfo is one container in a pod detail view.
type ContainerInfo struct {
	Name         string
	Image        string
	Ready        bool
	Restarts     int32
	State        string
	CPURequest   string
	CPULimit     string
	MemRequest   string
	MemLimit     string
}

// PodDetail is the full read-only pod detail.
type PodDetail struct {
	Name       string
	Namespace  string
	Phase      string
	Node       string
	PodIP      string
	Age        string
	Ready      string
	Containers []ContainerInfo
}

// Client talks to one environment's context.
type Client struct {
	cs kubernetes.Interface
}

var (
	clientCache   = map[string]*Client{}
	clientCacheMu sync.Mutex
)

// NewClient builds (or reuses) a client for the given environment.
func NewClient(env config.Environment) (*Client, error) {
	kubeconfig := config.ResolveKubeconfig(env)
	key := kubeconfig + "|" + env.Context
	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()
	if c, ok := clientCache[key]; ok {
		return c, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig

	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: env.Context,
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig for context %q: %w", env.Context, err)
	}
	restCfg.Timeout = defaultTimeout

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create client for context %q: %w", env.Context, err)
	}

	c := &Client{cs: cs}
	clientCache[key] = c
	return c, nil
}

// ContextExists reports whether the named context is present in the resolved kubeconfig.
func ContextExists(env config.Environment) bool {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = config.ResolveKubeconfig(env)
	raw, err := loadingRules.Load()
	if err != nil {
		return false
	}
	_, ok := raw.Contexts[env.Context]
	return ok
}

// ListNamespaces returns namespace summaries with deployment and pod counts.
func (c *Client) ListNamespaces(ctx context.Context) ([]NamespaceInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	nsList, err := c.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	depList, err := c.cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	podList, err := c.cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	depCount := map[string]int{}
	for _, d := range depList.Items {
		depCount[d.Namespace]++
	}

	readyPods := map[string]int{}
	totalPods := map[string]int{}
	for _, p := range podList.Items {
		totalPods[p.Namespace]++
		if isPodReady(p) {
			readyPods[p.Namespace]++
		}
	}

	out := make([]NamespaceInfo, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		out = append(out, NamespaceInfo{
			Name:            ns.Name,
			DeploymentCount: depCount[ns.Name],
			ReadyPods:       readyPods[ns.Name],
			TotalPods:       totalPods[ns.Name],
		})
	}
	return out, nil
}

// ListWorkloads returns deployments in a namespace.
func (c *Client) ListWorkloads(ctx context.Context, namespace string) ([]WorkloadInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	depList, err := c.cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments in %q: %w", namespace, err)
	}

	now := time.Now()
	out := make([]WorkloadInfo, 0, len(depList.Items))
	for _, d := range depList.Items {
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		out = append(out, WorkloadInfo{
			Name:      d.Name,
			Ready:     d.Status.ReadyReplicas,
			Desired:   desired,
			Available: d.Status.AvailableReplicas,
			Updated:   d.Status.UpdatedReplicas,
			Age:       formatAge(now.Sub(d.CreationTimestamp.Time)),
		})
	}
	return out, nil
}

// ListPods returns pods for a deployment in a namespace (via the deployment selector).
func (c *Client) ListPods(ctx context.Context, namespace, deploymentName string) ([]PodInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	dep, err := c.cs.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %q: %w", deploymentName, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("deployment selector: %w", err)
	}

	podList, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %q: %w", deploymentName, err)
	}

	now := time.Now()
	out := make([]PodInfo, 0, len(podList.Items))
	for _, p := range podList.Items {
		ready, total := podReadyCounts(p)
		cpuReq, cpuLim, memReq, memLim := PodResources(p)
		out = append(out, PodInfo{
			Name:       p.Name,
			Ready:      fmt.Sprintf("%d/%d", ready, total),
			Restarts:   podRestarts(p),
			Status:     podStatus(p),
			Node:       p.Spec.NodeName,
			Age:        formatAge(now.Sub(p.CreationTimestamp.Time)),
			CPURequest: cpuReq,
			CPULimit:   cpuLim,
			MemRequest: memReq,
			MemLimit:   memLim,
		})
	}
	return out, nil
}

// GetPod returns a detailed pod view.
func (c *Client) GetPod(ctx context.Context, namespace, name string) (*PodDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	p, err := c.cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %q: %w", name, err)
	}

	now := time.Now()
	ready, total := podReadyCounts(*p)
	statusByName := map[string]corev1.ContainerStatus{}
	for _, st := range p.Status.ContainerStatuses {
		statusByName[st.Name] = st
	}
	// Include init containers' statuses when present for restart counts on matching names.
	for _, st := range p.Status.InitContainerStatuses {
		if _, ok := statusByName[st.Name]; !ok {
			statusByName[st.Name] = st
		}
	}

	containers := make([]ContainerInfo, 0, len(p.Spec.Containers))
	for _, ctr := range p.Spec.Containers {
		info := ContainerInfo{
			Name:       ctr.Name,
			Image:      ctr.Image,
			CPURequest: resourceQuantity(ctr.Resources.Requests, corev1.ResourceCPU),
			CPULimit:   resourceQuantity(ctr.Resources.Limits, corev1.ResourceCPU),
			MemRequest: resourceQuantity(ctr.Resources.Requests, corev1.ResourceMemory),
			MemLimit:   resourceQuantity(ctr.Resources.Limits, corev1.ResourceMemory),
			State:      "Unknown",
		}
		if st, ok := statusByName[ctr.Name]; ok {
			info.Ready = st.Ready
			info.Restarts = st.RestartCount
			info.State = containerState(st)
		}
		containers = append(containers, info)
	}

	return &PodDetail{
		Name:       p.Name,
		Namespace:  p.Namespace,
		Phase:      string(p.Status.Phase),
		Node:       p.Spec.NodeName,
		PodIP:      p.Status.PodIP,
		Age:        formatAge(now.Sub(p.CreationTimestamp.Time)),
		Ready:      fmt.Sprintf("%d/%d", ready, total),
		Containers: containers,
	}, nil
}

// PodLogOptions builds PodLogOptions for a one-shot or follow request.
func PodLogOptions(container string, tailLines int, follow bool) *corev1.PodLogOptions {
	if tailLines <= 0 {
		tailLines = defaultTail
	}
	tail := int64(tailLines)
	return &corev1.PodLogOptions{
		Container: container,
		TailLines: &tail,
		Follow:    follow,
	}
}

// GetPodLogs returns the last tailLines of logs for a container (default 200).
func (c *Client) GetPodLogs(ctx context.Context, namespace, pod, container string, tailLines int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, logsTimeout)
	defer cancel()

	opts := PodLogOptions(container, tailLines, false)
	raw, err := c.cs.CoreV1().Pods(namespace).GetLogs(pod, opts).DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("logs for %s/%s: %w", pod, container, err)
	}
	return string(raw), nil
}

// FollowPodLogs opens a streaming log reader (TailLines then Follow).
// Caller must Close the reader and cancel ctx to stop the stream.
func (c *Client) FollowPodLogs(ctx context.Context, namespace, pod, container string, tailLines int) (io.ReadCloser, error) {
	opts := PodLogOptions(container, tailLines, true)
	stream, err := c.cs.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("follow logs for %s/%s: %w", pod, container, err)
	}
	return stream, nil
}

func resourceQuantity(list corev1.ResourceList, name corev1.ResourceName) string {
	if list == nil {
		return "-"
	}
	q, ok := list[name]
	if !ok {
		return "-"
	}
	return q.String()
}

// PodResources sums CPU/memory requests and limits across app containers (not init).
func PodResources(p corev1.Pod) (cpuReq, cpuLim, memReq, memLim string) {
	var cpuR, cpuL, memR, memL resource.Quantity
	hasCPUReq, hasCPULim, hasMemReq, hasMemLim := false, false, false, false
	for _, ctr := range p.Spec.Containers {
		if q, ok := ctr.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuR.Add(q)
			hasCPUReq = true
		}
		if q, ok := ctr.Resources.Limits[corev1.ResourceCPU]; ok {
			cpuL.Add(q)
			hasCPULim = true
		}
		if q, ok := ctr.Resources.Requests[corev1.ResourceMemory]; ok {
			memR.Add(q)
			hasMemReq = true
		}
		if q, ok := ctr.Resources.Limits[corev1.ResourceMemory]; ok {
			memL.Add(q)
			hasMemLim = true
		}
	}
	return quantityOrDash(cpuR, hasCPUReq), quantityOrDash(cpuL, hasCPULim),
		quantityOrDash(memR, hasMemReq), quantityOrDash(memL, hasMemLim)
}

func quantityOrDash(q resource.Quantity, set bool) string {
	if !set {
		return "-"
	}
	return q.String()
}

// IsForbidden reports whether err (or a wrapped cause) is Forbidden or Unauthorized.
func IsForbidden(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return true
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		c := status.Status().Code
		return c == 403 || c == 401
	}
	return false
}

func podReadyCounts(p corev1.Pod) (ready, total int) {
	total = len(p.Status.ContainerStatuses)
	if total == 0 {
		total = len(p.Spec.Containers)
	}
	for _, st := range p.Status.ContainerStatuses {
		if st.Ready {
			ready++
		}
	}
	return ready, total
}

func podRestarts(p corev1.Pod) int32 {
	var n int32
	for _, st := range p.Status.ContainerStatuses {
		n += st.RestartCount
	}
	return n
}

func podStatus(p corev1.Pod) string {
	for _, st := range p.Status.ContainerStatuses {
		if st.State.Waiting != nil && st.State.Waiting.Reason != "" {
			return st.State.Waiting.Reason
		}
		if st.State.Terminated != nil && st.State.Terminated.Reason != "" {
			return st.State.Terminated.Reason
		}
	}
	if p.Status.Phase != "" {
		return string(p.Status.Phase)
	}
	return "Unknown"
}

func containerState(st corev1.ContainerStatus) string {
	switch {
	case st.State.Running != nil:
		return "Running"
	case st.State.Waiting != nil:
		if st.State.Waiting.Reason != "" {
			return st.State.Waiting.Reason
		}
		return "Waiting"
	case st.State.Terminated != nil:
		if st.State.Terminated.Reason != "" {
			return st.State.Terminated.Reason
		}
		return "Terminated"
	default:
		return "Unknown"
	}
}

func isPodReady(p corev1.Pod) bool {
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// ClearClientCache drops cached clients (useful in tests).
func ClearClientCache() {
	clientCacheMu.Lock()
	defer clientCacheMu.Unlock()
	clientCache = map[string]*Client{}
}
