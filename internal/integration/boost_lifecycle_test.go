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
	"github.com/google/kube-startup-cpu-boost/internal/boost/duration"
	bpod "github.com/google/kube-startup-cpu-boost/internal/boost/pod"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	apiResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Boost Lifecycle", func() {
	var (
		ns       *corev1.Namespace
		nsName   string
		boostObj *autoscaling.StartupCPUBoost
	)

	BeforeEach(func() {
		nsName = fmt.Sprintf("test-lifecycle-%d", time.Now().UnixNano())
		ns = createNamespace(ctx, nsName, nil)
	})

	AfterEach(func() {
		// Clean up boost if it exists
		if boostObj != nil {
			_ = k8sClient.Delete(ctx, boostObj)
		}
		_ = k8sClient.Delete(ctx, ns)
	})

	Describe("creating a StartupCPUBoost", func() {
		BeforeEach(func() {
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-boost",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "test-app"),
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
		})

		It("sets Active=True status condition", func() {
			Eventually(func(g Gomega) {
				var fetched autoscaling.StartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: boostObj.Name, Namespace: nsName,
				}, &fetched)).To(Succeed())
				cond := meta.FindStatusCondition(fetched.Status.Conditions, "Active")
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal("Ready"))
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("registers the boost in the manager", func() {
			Eventually(func() bool {
				_, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				return ok
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("pod tracking", func() {
		var pod *corev1.Pod

		BeforeEach(func() {
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-boost-track",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "track-app"),
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

			reqQuantity := apiResource.MustParse("250m")
			limitQuantity := apiResource.MustParse("500m")
			annot := &bpod.BoostPodAnnotation{
				BoostTimestamp:  time.Now(),
				InitCPURequests: map[string]string{"main": "250m"},
				InitCPULimits:   map[string]string{"main": "500m"},
			}
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tracked-pod",
					Namespace: nsName,
					Labels: map[string]string{
						"app":              "track-app",
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
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: limitQuantity,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		})

		It("tracks the labeled pod in the boost", func() {
			Eventually(func() bool {
				b, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				if !ok {
					return false
				}
				_, podOk := b.Pod(pod.Name)
				return podOk
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		})

		It("updates status with active container boosts", func() {
			Eventually(func(g Gomega) {
				var fetched autoscaling.StartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: boostObj.Name, Namespace: nsName,
				}, &fetched)).To(Succeed())
				g.Expect(fetched.Status.ActiveContainerBoosts).To(BeNumerically(">=", int32(1)))
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})
	})

	Describe("deleting a StartupCPUBoost", func() {
		BeforeEach(func() {
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-boost-delete",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "delete-app"),
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
			Expect(k8sClient.Create(ctx, boostObj)).To(Succeed())

			// Wait for boost to be registered
			Eventually(func() bool {
				_, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				return ok
			}, 10*time.Second, 250*time.Millisecond).Should(BeTrue())
		})

		It("removes the boost from the manager", func() {
			Expect(k8sClient.Delete(ctx, boostObj)).To(Succeed())
			Eventually(func() bool {
				_, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				return ok
			}, 10*time.Second, 250*time.Millisecond).Should(BeFalse())
			boostObj = nil // prevent AfterEach double-delete
		})
	})

	Describe("boost with fixed duration policy", func() {
		BeforeEach(func() {
			boostObj = &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-boost-timed",
					Namespace: nsName,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "timed-app"),
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
						Fixed: &autoscaling.FixedDurationPolicy{
							Unit:  autoscaling.FixedDurationPolicyUnitSec,
							Value: 60,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, boostObj)).To(Succeed())
		})

		It("registers the boost with a fixed duration policy in the manager", func() {
			Eventually(func(g Gomega) {
				b, ok := boostMgr.GetRegularCPUBoost(ctx, boostObj.Name, nsName)
				g.Expect(ok).To(BeTrue())
				_, hasFDP := b.DurationPolicies()[duration.FixedDurationPolicyName]
				g.Expect(hasFDP).To(BeTrue())
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})
	})
})
