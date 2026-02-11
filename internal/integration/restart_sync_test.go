//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration_test

import (
	"fmt"
	"time"

	autoscaling "github.com/google/kube-startup-cpu-boost/api/v1alpha1"
	bpod "github.com/google/kube-startup-cpu-boost/internal/boost/pod"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Restart Sync", func() {
	var (
		ns       *corev1.Namespace
		nsName   string
		boostObj *autoscaling.StartupCPUBoost
	)

	BeforeEach(func() {
		nsName = fmt.Sprintf("test-restart-%d", time.Now().UnixNano())
		ns = createNamespace(ctx, nsName, nil)
	})

	AfterEach(func() {
		if boostObj != nil {
			_ = k8sClient.Delete(ctx, boostObj)
		}
		_ = k8sClient.Delete(ctx, ns)
	})

	Describe("re-syncs pods when boost is re-registered", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sync-boost",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "sync-app"),
				Spec: autoscaling.StartupCPUBoostSpec{
					ResourcePolicy: autoscaling.ResourcePolicy{
						ContainerPolicies: []autoscaling.ContainerPolicy{
							{
								ContainerName: "main",
								PercentageIncrease: &autoscaling.PercentageIncrease{
									Value: 100,
								},
							},
						},
					},
					DurationPolicy: autoscaling.DurationPolicy{
						PodCondition: &autoscaling.PodConditionDurationPolicy{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, boostObj)).To(Succeed())

			// Wait for boost to be registered
			Eventually(func() bool {
				_, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				return ok
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			// Create a labeled pod
			reqQuantity := apiResource.MustParse("250m")
			annot := &bpod.BoostPodAnnotation{
				BoostTimestamp:  time.Now(),
				InitCPURequests: map[string]string{"main": "250m"},
				InitCPULimits:   map[string]string{},
			}
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sync-pod",
					Namespace: nsName,
					Labels: map[string]string{
						"app":              "sync-app",
						bpod.BoostLabelKey: boostObj.Name,
					},
					Annotations: map[string]string{
						bpod.BoostAnnotationKey: annot.ToJSON(),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "busybox:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: reqQuantity,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			// Verify pod is tracked
			Eventually(func() bool {
				b, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				if !ok {
					return false
				}
				_, podOk := b.Pod(pod.Name)
				return podOk
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		})

		It("re-tracks the pod after boost is deleted and recreated", func() {
			// Simulate controller restart: delete the boost (clears tracking)
			// then recreate it (triggers postProcessNewBoost -> syncBoostPods)
			Expect(k8sClient.Delete(ctx, boostObj)).To(Succeed())

			// Wait for boost to be deregistered
			Eventually(func() bool {
				_, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				return ok
			}, 10*time.Second, 250*time.Millisecond).Should(BeFalse())

			// Recreate the boost (simulating informer synthetic create on restart)
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sync-boost",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "sync-app"),
				Spec: autoscaling.StartupCPUBoostSpec{
					ResourcePolicy: autoscaling.ResourcePolicy{
						ContainerPolicies: []autoscaling.ContainerPolicy{
							{
								ContainerName: "main",
								PercentageIncrease: &autoscaling.PercentageIncrease{
									Value: 100,
								},
							},
						},
					},
					DurationPolicy: autoscaling.DurationPolicy{
						PodCondition: &autoscaling.PodConditionDurationPolicy{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, boostObj)).To(Succeed())

			// The syncBoostPods method should list the labeled pod from the API
			// and re-register it in the boost's pod tracking map
			Eventually(func() bool {
				b, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				if !ok {
					return false
				}
				_, podOk := b.Pod(pod.Name)
				return podOk
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("handles pods with no matching boost gracefully", func() {
		It("stores pod as orphaned when boost does not exist yet", func() {
			reqQuantity := apiResource.MustParse("250m")
			annot := &bpod.BoostPodAnnotation{
				BoostTimestamp:  time.Now(),
				InitCPURequests: map[string]string{"main": "250m"},
				InitCPULimits:   map[string]string{},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-pod",
					Namespace: nsName,
					Labels: map[string]string{
						"app":              "orphan-app",
						bpod.BoostLabelKey: "nonexistent-boost",
					},
					Annotations: map[string]string{
						bpod.BoostAnnotationKey: annot.ToJSON(),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "busybox:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: reqQuantity,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			// Create the boost after the pod — orphan mapping should match it
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "late-boost",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "orphan-app"),
				Spec: autoscaling.StartupCPUBoostSpec{
					ResourcePolicy: autoscaling.ResourcePolicy{
						ContainerPolicies: []autoscaling.ContainerPolicy{
							{
								ContainerName: "main",
								PercentageIncrease: &autoscaling.PercentageIncrease{
									Value: 50,
								},
							},
						},
					},
					DurationPolicy: autoscaling.DurationPolicy{
						PodCondition: &autoscaling.PodConditionDurationPolicy{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			// Give informer time to process the pod event first
			time.Sleep(1 * time.Second)

			Expect(k8sClient.Create(ctx, boostObj)).To(Succeed())

			// The orphan mapping or syncBoostPods should re-track the pod
			Eventually(func() bool {
				b, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				if !ok {
					return false
				}
				_, podOk := b.Pod(pod.Name)
				return podOk
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())

			_ = k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0))
		})
	})
})
