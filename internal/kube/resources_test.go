package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodResourcesSumsContainers(test *testing.T) {
	pod := corev1.Pod{
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
	cpuReq, cpuLim, memReq, memLim := PodResources(pod)
	if cpuReq != "150m" {
		test.Fatalf("cpuReq = %q", cpuReq)
	}
	if cpuLim != "300m" {
		test.Fatalf("cpuLim = %q", cpuLim)
	}
	if memReq != "192Mi" {
		test.Fatalf("memReq = %q", memReq)
	}
	if memLim != "384Mi" {
		test.Fatalf("memLim = %q", memLim)
	}
}

func TestPodResourcesEmpty(test *testing.T) {
	cpuReq, cpuLim, memReq, memLim := PodResources(corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "x"}}},
	})
	if cpuReq != "-" || cpuLim != "-" || memReq != "-" || memLim != "-" {
		test.Fatalf("got %s %s %s %s", cpuReq, cpuLim, memReq, memLim)
	}
}
