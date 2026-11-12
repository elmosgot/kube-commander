package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodResourcesSumsContainers(t *testing.T) {
	p := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "a",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
				{
					Name: "b",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
				},
			},
			InitContainers: []corev1.Container{
				{
					Name: "init",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
	cpuReq, cpuLim, memReq, memLim := PodResources(p)
	if cpuReq != "150m" {
		t.Fatalf("cpuReq = %q", cpuReq)
	}
	if cpuLim != "300m" {
		t.Fatalf("cpuLim = %q", cpuLim)
	}
	if memReq != "192Mi" {
		t.Fatalf("memReq = %q", memReq)
	}
	if memLim != "384Mi" {
		t.Fatalf("memLim = %q", memLim)
	}
}

func TestPodResourcesEmpty(t *testing.T) {
	cpuReq, cpuLim, memReq, memLim := PodResources(corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "x"}}},
	})
	if cpuReq != "-" || cpuLim != "-" || memReq != "-" || memLim != "-" {
		t.Fatalf("got %s %s %s %s", cpuReq, cpuLim, memReq, memLim)
	}
}
