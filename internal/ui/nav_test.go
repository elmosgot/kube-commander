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

func TestBackFromWorkloadsReturnsToLeft(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.leftCursor = 1
	model.focus = paneRight
	model.rightView = viewWorkloads
	model.namespace = tApp
	model.workloads = []kube.WorkloadInfo{{Name: tAPI}}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.focus != paneLeft {
		test.Fatalf("focus = %v want left", model.focus)
	}
	if model.loading {
		test.Fatal("should not load namespaces on back")
	}
}

func TestNamespacesForbiddenReturnsToLeft(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.leftCursor = 1
	model.focus = paneRight
	model.loading = true

	status := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   403,
		Reason: metav1.StatusReasonForbidden,
	}}
	updated, _ := model.Update(namespacesMsg{
		poolIdx: 0,
		envIdx:  0,
		err:     fmt.Errorf("list namespaces: %w", status),
	})
	model = updated.(Model)
	if model.focus != paneLeft {
		test.Fatalf("focus = %v", model.focus)
	}
	if !strings.Contains(model.err, "environments[].namespace") {
		test.Fatalf("err = %q", model.err)
	}
	if !kube.IsForbidden(fmt.Errorf("list namespaces: %w", status)) {
		test.Fatal("expected IsForbidden")
	}
}

func TestRenderPodRowsIncludesResources(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.pods = []kube.PodInfo{{
		Name: tAPI1, Ready: tReady11, Restarts: 0, Status: tRunning, Age: "1h",
		CPURequest: "100m", CPULimit: "200m", MemRequest: "128Mi", MemLimit: "256Mi",
	}}
	var b strings.Builder
	model.renderPodRows(&b, 160)
	got := b.String()
	if !strings.Contains(got, "100m/200m") {
		test.Fatalf("missing cpu resources:\n%s", got)
	}
	if !strings.Contains(got, "128Mi/256Mi") {
		test.Fatalf("missing mem resources:\n%s", got)
	}
}
