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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("GlobalStartupCPUBoost", func() {
	var (
		ns1Name        string
		ns2Name        string
		globalBoostObj *autoscaling.GlobalStartupCPUBoost
	)

	BeforeEach(func() {
		ts := time.Now().UnixNano()
		ns1Name = fmt.Sprintf("test-global-ns1-%d", ts)
		ns2Name = fmt.Sprintf("test-global-ns2-%d", ts)
	})

	AfterEach(func() {
		if globalBoostObj != nil {
			_ = k8sClient.Delete(ctx, globalBoostObj)
			// Wait for finalizer to clean up children
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: globalBoostObj.Name}, globalBoostObj)
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
			globalBoostObj = nil
		}
		for _, nsName := range []string{ns1Name, ns2Name} {
			ns := &corev1.Namespace{}
			ns.Name = nsName
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	Describe("creating child boosts in matching namespaces", func() {
		BeforeEach(func() {
			createNamespace(ctx, ns1Name, map[string]string{"env": "staging"})
			createNamespace(ctx, ns2Name, map[string]string{"env": "staging"})

			globalBoostObj = &autoscaling.GlobalStartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("global-boost-%d", time.Now().UnixNano()),
				},
				Spec: autoscaling.GlobalStartupCPUBoostSpec{
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "staging"},
					},
					Template: autoscaling.GlobalStartupCPUBoostTemplate{
						Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
							"app", "global-test"),
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, globalBoostObj)).To(Succeed())
		})

		It("creates child StartupCPUBoost in each matching namespace", func() {
			childName := autoscaling.GlobalBoostChildPrefix + globalBoostObj.Name
			for _, nsName := range []string{ns1Name, ns2Name} {
				nsName := nsName
				Eventually(func(g Gomega) {
					var child autoscaling.StartupCPUBoost
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name: childName, Namespace: nsName,
					}, &child)).To(Succeed())
					g.Expect(child.Labels[autoscaling.ManagedByGlobalBoostLabel]).To(Equal(globalBoostObj.Name))
				}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
			}
		})

		It("sets Active status condition on the global boost", func() {
			Eventually(func(g Gomega) {
				var fetched autoscaling.GlobalStartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: globalBoostObj.Name,
				}, &fetched)).To(Succeed())
				g.Expect(len(fetched.Status.ActiveNamespaces)).To(BeNumerically(">=", 2))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	Describe("reacting to new namespace creation", func() {
		BeforeEach(func() {
			// Create global boost first, before any matching namespace
			globalBoostObj = &autoscaling.GlobalStartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("global-ns-watch-%d", time.Now().UnixNano()),
				},
				Spec: autoscaling.GlobalStartupCPUBoostSpec{
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "watch-test"},
					},
					Template: autoscaling.GlobalStartupCPUBoostTemplate{
						Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
							"app", "watch-app"),
						Spec: autoscaling.StartupCPUBoostSpec{
							ResourcePolicy: autoscaling.ResourcePolicy{
								ContainerPolicies: []autoscaling.ContainerPolicy{
									{
										ContainerName: "main",
										PercentageIncrease: &autoscaling.PercentageIncrease{
											Value: 75,
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, globalBoostObj)).To(Succeed())

			// Wait for the global boost to be reconciled
			Eventually(func(g Gomega) {
				var fetched autoscaling.GlobalStartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: globalBoostObj.Name,
				}, &fetched)).To(Succeed())
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("creates a child boost when a matching namespace is created", func() {
			// Create a namespace that matches the selector
			createNamespace(ctx, ns1Name, map[string]string{"env": "watch-test"})

			childName := autoscaling.GlobalBoostChildPrefix + globalBoostObj.Name
			Eventually(func(g Gomega) {
				var child autoscaling.StartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: childName, Namespace: ns1Name,
				}, &child)).To(Succeed())
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	Describe("deletion cleanup via finalizer", func() {
		BeforeEach(func() {
			createNamespace(ctx, ns1Name, map[string]string{"env": "cleanup"})

			globalBoostObj = &autoscaling.GlobalStartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("global-cleanup-%d", time.Now().UnixNano()),
				},
				Spec: autoscaling.GlobalStartupCPUBoostSpec{
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "cleanup"},
					},
					Template: autoscaling.GlobalStartupCPUBoostTemplate{
						Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
							"app", "cleanup-app"),
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, globalBoostObj)).To(Succeed())

			// Wait for child to be created
			childName := autoscaling.GlobalBoostChildPrefix + globalBoostObj.Name
			Eventually(func(g Gomega) {
				var child autoscaling.StartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: childName, Namespace: ns1Name,
				}, &child)).To(Succeed())
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
		})

		It("removes all child boosts when global boost is deleted", func() {
			childName := autoscaling.GlobalBoostChildPrefix + globalBoostObj.Name
			Expect(k8sClient.Delete(ctx, globalBoostObj)).To(Succeed())

			// Verify child is cleaned up
			Eventually(func() bool {
				var child autoscaling.StartupCPUBoost
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: childName, Namespace: ns1Name,
				}, &child)
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

			// Verify the global boost itself is deleted (finalizer processed)
			Eventually(func() bool {
				var fetched autoscaling.GlobalStartupCPUBoost
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: globalBoostObj.Name,
				}, &fetched)
				return err != nil
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

			globalBoostObj = nil // prevent AfterEach double-delete
		})
	})

	Describe("skipping namespaces with unmanaged boosts", func() {
		BeforeEach(func() {
			createNamespace(ctx, ns1Name, map[string]string{"env": "skip-test"})

			// Create a user-managed boost in the namespace first
			userBoost := &autoscaling.StartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-boost",
					Namespace: ns1Name,
				},
				Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
					"app", "skip-app"),
				Spec: autoscaling.StartupCPUBoostSpec{
					ResourcePolicy: autoscaling.ResourcePolicy{
						ContainerPolicies: []autoscaling.ContainerPolicy{
							{
								ContainerName: "main",
								PercentageIncrease: &autoscaling.PercentageIncrease{
									Value: 25,
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
			Expect(k8sClient.Create(ctx, userBoost)).To(Succeed())
		})

		It("reports skipped namespaces in status", func() {
			globalBoostObj = &autoscaling.GlobalStartupCPUBoost{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("global-skip-%d", time.Now().UnixNano()),
				},
				Spec: autoscaling.GlobalStartupCPUBoostSpec{
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "skip-test"},
					},
					Template: autoscaling.GlobalStartupCPUBoostTemplate{
						Selector: *metav1.AddLabelToSelector(&metav1.LabelSelector{},
							"app", "skip-app"),
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, globalBoostObj)).To(Succeed())

			Eventually(func(g Gomega) {
				var fetched autoscaling.GlobalStartupCPUBoost
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: globalBoostObj.Name,
				}, &fetched)).To(Succeed())
				g.Expect(len(fetched.Status.SkippedNamespaces)).To(BeNumerically(">=", 1))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})
})
