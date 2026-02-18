// Copyright 2024 Google LLC
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

package webhook_test

import (
	"context"

	"github.com/google/kube-startup-cpu-boost/api/v1alpha1"
	"github.com/google/kube-startup-cpu-boost/internal/webhook"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GlobalStartupCPUBoost webhook", func() {
	var w webhook.GlobalStartupCPUBoostWebhook
	BeforeEach(func() {
		w = webhook.GlobalStartupCPUBoostWebhook{}
	})

	When("Validates GlobalStartupCPUBoost", func() {
		var (
			boost v1alpha1.GlobalStartupCPUBoost
			err   error
		)

		When("valid GlobalStartupCPUBoost with percentage increase and fixed duration", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-001",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 100},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("does not error", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).NotTo(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("valid GlobalStartupCPUBoost with MatchExpressions in selectors", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-matchexpr",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "env",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"prod", "staging"},
								},
							},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpExists,
										Values:   []string{},
									},
								},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 100},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("does not error", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).NotTo(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has invalid MatchExpressions in namespace selector", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-invalid-ns-expr",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "env",
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{},
								},
							},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 100},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has invalid MatchExpressions in pod selector", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-invalid-pod-expr",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{},
									},
								},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 100},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has no duration policy", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-no-duration",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      "container-one",
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 50},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has container with two resource policies", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-two-policies",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      "container-one",
											FixedResources:     &v1alpha1.FixedResources{},
											PercentageIncrease: &v1alpha1.PercentageIncrease{},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has duplicate wildcard container policies", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-dup-wildcard",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 50},
										},
										{
											ContainerName:  v1alpha1.ContainerPolicyWildcard,
											FixedResources: &v1alpha1.FixedResources{},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed: &v1alpha1.FixedDurationPolicy{
										Value: 60,
										Unit:  v1alpha1.FixedDurationPolicyUnitSec,
									},
								},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("GlobalStartupCPUBoost has both duration policies", func() {
			BeforeEach(func() {
				boost = v1alpha1.GlobalStartupCPUBoost{
					ObjectMeta: metav1.ObjectMeta{
						Name: "global-boost-both-duration",
					},
					Spec: v1alpha1.GlobalStartupCPUBoostSpec{
						NamespaceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Template: v1alpha1.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "java"},
							},
							Spec: v1alpha1.StartupCPUBoostSpec{
								ResourcePolicy: v1alpha1.ResourcePolicy{
									ContainerPolicies: []v1alpha1.ContainerPolicy{
										{
											ContainerName:      v1alpha1.ContainerPolicyWildcard,
											PercentageIncrease: &v1alpha1.PercentageIncrease{Value: 50},
										},
									},
								},
								DurationPolicy: v1alpha1.DurationPolicy{
									Fixed:        &v1alpha1.FixedDurationPolicy{},
									PodCondition: &v1alpha1.PodConditionDurationPolicy{},
								},
							},
						},
					},
				}
			})
			It("errors", func() {
				By("validating create event")
				_, err = w.ValidateCreate(context.TODO(), &boost)
				Expect(err).To(HaveOccurred())

				By("validating update event")
				_, err = w.ValidateUpdate(context.TODO(), nil, &boost)
				Expect(err).To(HaveOccurred())
			})
		})

		When("ValidateDelete is called", func() {
			It("does not error", func() {
				_, err = w.ValidateDelete(context.TODO(), &boost)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
