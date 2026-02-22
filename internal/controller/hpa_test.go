// Package controller implements the reconciliation logic for the memcached-operator.
package controller

import (
	"reflect"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	memcachedv1beta1 "github.com/c5c3/memcached-operator/api/v1beta1"
)

func TestHpaEnabled(t *testing.T) {
	tests := []struct {
		name string
		mc   *memcachedv1beta1.Memcached
		want bool
	}{
		{
			name: "nil Autoscaling",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{Autoscaling: nil},
			},
			want: false,
		},
		{
			name: "Autoscaling with Enabled=false",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{Enabled: false},
				},
			},
			want: false,
		},
		{
			name: "Autoscaling with Enabled=true",
			mc: &memcachedv1beta1.Memcached{
				Spec: memcachedv1beta1.MemcachedSpec{
					Autoscaling: &memcachedv1beta1.AutoscalingSpec{Enabled: true},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hpaEnabled(tt.mc)
			if got != tt.want {
				t.Errorf("hpaEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstructHPA(t *testing.T) {
	minReplicas := int32(2)

	tests := []struct {
		name            string
		autoscaling     *memcachedv1beta1.AutoscalingSpec
		wantMinReplicas *int32
		wantMaxReplicas int32
		wantMetrics     []autoscalingv2.MetricSpec
		wantBehavior    *autoscalingv2.HorizontalPodAutoscalerBehavior
	}{
		{
			name: "basic with minReplicas and maxReplicas",
			autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &minReplicas,
				MaxReplicas: 10,
			},
			wantMinReplicas: &minReplicas,
			wantMaxReplicas: 10,
			wantMetrics:     nil,
			wantBehavior:    nil,
		},
		{
			name: "with metrics",
			autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &minReplicas,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: int32Ptr(80),
							},
						},
					},
				},
			},
			wantMinReplicas: &minReplicas,
			wantMaxReplicas: 10,
			wantMetrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: v1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: int32Ptr(80),
						},
					},
				},
			},
			wantBehavior: nil,
		},
		{
			name: "with behavior",
			autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &minReplicas,
				MaxReplicas: 10,
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: int32Ptr(300),
					},
				},
			},
			wantMinReplicas: &minReplicas,
			wantMaxReplicas: 10,
			wantMetrics:     nil,
			wantBehavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: int32Ptr(300),
				},
			},
		},
		{
			name: "nil minReplicas",
			autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MaxReplicas: 5,
			},
			wantMinReplicas: nil,
			wantMaxReplicas: 5,
			wantMetrics:     nil,
			wantBehavior:    nil,
		},
		{
			name: "full configuration",
			autoscaling: &memcachedv1beta1.AutoscalingSpec{
				Enabled:     true,
				MinReplicas: &minReplicas,
				MaxReplicas: 20,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: int32Ptr(70),
							},
						},
					},
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: v1.ResourceMemory,
							Target: autoscalingv2.MetricTarget{
								Type:         autoscalingv2.AverageValueMetricType,
								AverageValue: resourceQuantityPtr(resource.MustParse("500Mi")),
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: int32Ptr(300),
					},
					ScaleUp: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: int32Ptr(60),
					},
				},
			},
			wantMinReplicas: &minReplicas,
			wantMaxReplicas: 20,
			wantMetrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: v1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: int32Ptr(70),
						},
					},
				},
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: v1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: resourceQuantityPtr(resource.MustParse("500Mi")),
						},
					},
				},
			},
			wantBehavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: int32Ptr(300),
				},
				ScaleUp: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: int32Ptr(60),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &memcachedv1beta1.Memcached{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-cache",
					Namespace: "default",
				},
				Spec: memcachedv1beta1.MemcachedSpec{
					Autoscaling: tt.autoscaling,
				},
			}
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}

			constructHPA(mc, hpa)

			// Verify labels.
			expectedLabels := labelsForMemcached("my-cache")
			if len(hpa.Labels) != len(expectedLabels) {
				t.Errorf("expected %d labels, got %d", len(expectedLabels), len(hpa.Labels))
			}
			for k, v := range expectedLabels {
				if hpa.Labels[k] != v {
					t.Errorf("label %q = %q, want %q", k, hpa.Labels[k], v)
				}
			}

			// Verify ScaleTargetRef.
			wantRef := autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "my-cache",
			}
			if hpa.Spec.ScaleTargetRef != wantRef {
				t.Errorf("ScaleTargetRef = %+v, want %+v", hpa.Spec.ScaleTargetRef, wantRef)
			}

			// Verify MinReplicas.
			if tt.wantMinReplicas == nil {
				if hpa.Spec.MinReplicas != nil {
					t.Errorf("expected nil MinReplicas, got %v", *hpa.Spec.MinReplicas)
				}
			} else {
				if hpa.Spec.MinReplicas == nil {
					t.Fatalf("expected MinReplicas %d, got nil", *tt.wantMinReplicas)
				}
				if *hpa.Spec.MinReplicas != *tt.wantMinReplicas {
					t.Errorf("MinReplicas = %d, want %d", *hpa.Spec.MinReplicas, *tt.wantMinReplicas)
				}
			}

			// Verify MaxReplicas.
			if hpa.Spec.MaxReplicas != tt.wantMaxReplicas {
				t.Errorf("MaxReplicas = %d, want %d", hpa.Spec.MaxReplicas, tt.wantMaxReplicas)
			}

			// Verify Metrics.
			if !reflect.DeepEqual(hpa.Spec.Metrics, tt.wantMetrics) {
				t.Errorf("Metrics = %+v, want %+v", hpa.Spec.Metrics, tt.wantMetrics)
			}

			// Verify Behavior.
			if !reflect.DeepEqual(hpa.Spec.Behavior, tt.wantBehavior) {
				t.Errorf("Behavior = %+v, want %+v", hpa.Spec.Behavior, tt.wantBehavior)
			}
		})
	}
}

func resourceQuantityPtr(q resource.Quantity) *resource.Quantity {
	return &q
}
