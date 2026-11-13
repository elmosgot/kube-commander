package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elmosgot/kube-commander/internal/kube"
)

func TestBackFromWorkloadsReturnsToLeft(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.leftCursor = 1
	m.focus = paneRight
	m.rightView = viewWorkloads
	m.namespace = tApp
	m.workloads = []kube.WorkloadInfo{{Name: tAPI}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.focus != paneLeft {
		t.Fatalf("focus = %v want left", m.focus)
	}
	if m.loading {
		t.Fatal("should not load namespaces on back")
	}
}

func TestNamespacesForbiddenReturnsToLeft(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.leftCursor = 1
	m.focus = paneRight
	m.loading = true

	status := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   403,
		Reason: metav1.StatusReasonForbidden,
	}}
	updated, _ := m.Update(namespacesMsg{
		poolIdx: 0,
		envIdx:  0,
		err:     fmt.Errorf("list namespaces: %w", status),
	})
	m = updated.(Model)
	if m.focus != paneLeft {
		t.Fatalf("focus = %v", m.focus)
	}
	if !strings.Contains(m.err, "environments[].namespace") {
		t.Fatalf("err = %q", m.err)
	}
	if !kube.IsForbidden(fmt.Errorf("list namespaces: %w", status)) {
		t.Fatal("expected IsForbidden")
	}
}

func TestRenderPodRowsIncludesResources(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.pods = []kube.PodInfo{{
		Name: tAPI1, Ready: tReady11, Restarts: 0, Status: "Running", Age: "1h",
		CPURequest: "100m", CPULimit: "200m", MemRequest: "128Mi", MemLimit: "256Mi",
	}}
	var b strings.Builder
	m.renderPodRows(&b, 160)
	got := b.String()
	if !strings.Contains(got, "100m/200m") {
		t.Fatalf("missing cpu resources:\n%s", got)
	}
	if !strings.Contains(got, "128Mi/256Mi") {
		t.Fatalf("missing mem resources:\n%s", got)
	}
}
