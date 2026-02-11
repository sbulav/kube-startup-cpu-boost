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

package controller_test

import (
	"context"

	"github.com/go-logr/logr"
	autoscaling "github.com/google/kube-startup-cpu-boost/api/v1alpha1"
	"github.com/google/kube-startup-cpu-boost/internal/controller"
	"github.com/google/kube-startup-cpu-boost/internal/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("GlobalBoostController", func() {
	var (
		mockCtrl   *gomock.Controller
		mockClient *mock.MockClient
		reconciler controller.GlobalStartupCPUBoostReconciler
		testScheme *runtime.Scheme
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mock.NewMockClient(mockCtrl)
		testScheme = runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
		utilruntime.Must(autoscaling.AddToScheme(testScheme))
		reconciler = controller.GlobalStartupCPUBoostReconciler{
			Log:    logr.Discard(),
			Client: mockClient,
			Scheme: testScheme,
		}
	})

	Describe("Setups with manager", func() {
		var (
			mockCtrlManager *mock.MockCtrlManager
			err             error
		)
		BeforeEach(func() {
			mockCtrlManager = mock.NewMockCtrlManager(mockCtrl)
			skipNameValidation := true
			mockCtrlManager.EXPECT().GetControllerOptions().
				Return(config.Controller{SkipNameValidation: &skipNameValidation}).MinTimes(1)
			mockCtrlManager.EXPECT().GetScheme().Return(testScheme).MinTimes(1)
			mockCtrlManager.EXPECT().GetLogger().Return(logr.Discard()).MinTimes(1)
			mockCtrlManager.EXPECT().Add(gomock.Any()).Return(nil).MinTimes(1)
			mockCtrlManager.EXPECT().GetCache().Return(&informertest.FakeInformers{}).MinTimes(1)
		})
		JustBeforeEach(func() {
			err = reconciler.SetupWithManager(mockCtrlManager)
		})
		It("doesn't error", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Receives reconcile request", func() {
		var (
			req    ctrl.Request
			name   string
			result ctrl.Result
			err    error
		)
		BeforeEach(func() {
			name = "global-boost-001"
			req = ctrl.Request{
				NamespacedName: types.NamespacedName{Name: name},
			}
		})
		JustBeforeEach(func() {
			result, err = reconciler.Reconcile(context.TODO(), req)
		})

		When("GlobalStartupCPUBoost is not found", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(req.NamespacedName), gomock.Any()).
					Return(apierrors.NewNotFound(schema.GroupResource{}, name))
			})
			It("does not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("returns empty result", func() {
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		When("GlobalStartupCPUBoost exists without finalizer", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(req.NamespacedName), gomock.Any()).
					DoAndReturn(func(c context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						gb := obj.(*autoscaling.GlobalStartupCPUBoost)
						gb.Name = name
						gb.Spec.NamespaceSelector = metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "test"},
						}
						gb.Spec.Template = autoscaling.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "demo"},
							},
							Spec: autoscaling.StartupCPUBoostSpec{
								DurationPolicy: autoscaling.DurationPolicy{
									Fixed: &autoscaling.FixedDurationPolicy{
										Value: 60,
										Unit:  autoscaling.FixedDurationPolicyUnitSec,
									},
								},
							},
						}
						return nil
					})
				// Expect the finalizer to be added
				mockClient.EXPECT().Update(gomock.Any(), gomock.Cond(func(obj any) bool {
					gb := obj.(*autoscaling.GlobalStartupCPUBoost)
					return controllerutil.ContainsFinalizer(gb, autoscaling.GlobalBoostFinalizer)
				})).Return(nil)
			})
			It("adds the finalizer and requeues", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
			})
		})

		When("GlobalStartupCPUBoost is being deleted", func() {
			BeforeEach(func() {
				now := metav1.Now()
				mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(req.NamespacedName), gomock.Any()).
					DoAndReturn(func(c context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						gb := obj.(*autoscaling.GlobalStartupCPUBoost)
						gb.Name = name
						gb.DeletionTimestamp = &now
						gb.Finalizers = []string{autoscaling.GlobalBoostFinalizer}
						return nil
					})
				// Expect listing of managed children
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&autoscaling.StartupCPUBoostList{}),
					gomock.Any()).
					DoAndReturn(func(c context.Context, list client.ObjectList, opts ...client.ListOption) error {
						// No children to clean up
						return nil
					})
				// Expect finalizer removal
				mockClient.EXPECT().Update(gomock.Any(), gomock.Cond(func(obj any) bool {
					gb := obj.(*autoscaling.GlobalStartupCPUBoost)
					return !controllerutil.ContainsFinalizer(gb, autoscaling.GlobalBoostFinalizer)
				})).Return(nil)
			})
			It("removes the finalizer", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		When("GlobalStartupCPUBoost has finalizer and matching namespaces", func() {
			var mockSubResClient *mock.MockSubResourceClient
			BeforeEach(func() {
				mockSubResClient = mock.NewMockSubResourceClient(mockCtrl)
				mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(req.NamespacedName), gomock.Any()).
					DoAndReturn(func(c context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						gb := obj.(*autoscaling.GlobalStartupCPUBoost)
						gb.Name = name
						gb.UID = "test-uid"
						gb.Finalizers = []string{autoscaling.GlobalBoostFinalizer}
						gb.Spec.NamespaceSelector = metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "test"},
						}
						gb.Spec.Template = autoscaling.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "demo"},
							},
							Spec: autoscaling.StartupCPUBoostSpec{
								ResourcePolicy: autoscaling.ResourcePolicy{
									ContainerPolicies: []autoscaling.ContainerPolicy{
										{
											ContainerName:      autoscaling.ContainerPolicyWildcard,
											PercentageIncrease: &autoscaling.PercentageIncrease{Value: 100},
										},
									},
								},
								DurationPolicy: autoscaling.DurationPolicy{
									Fixed: &autoscaling.FixedDurationPolicy{
										Value: 60,
										Unit:  autoscaling.FixedDurationPolicyUnitSec,
									},
								},
							},
						}
						return nil
					})

				// List namespaces matching selector
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NamespaceList{}), gomock.Any()).
					DoAndReturn(func(c context.Context, list client.ObjectList, opts ...client.ListOption) error {
						nsList := list.(*corev1.NamespaceList)
						nsList.Items = []corev1.Namespace{
							{ObjectMeta: metav1.ObjectMeta{Name: "ns-a", Labels: map[string]string{"env": "test"}}},
						}
						return nil
					})

				// Check for unmanaged boosts in ns-a (none)
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&autoscaling.StartupCPUBoostList{}),
					gomock.Cond(func(opts any) bool {
						// Match the List call for checking unmanaged boosts
						return true
					})).
					DoAndReturn(func(c context.Context, list client.ObjectList, opts ...client.ListOption) error {
						return nil
					}).MinTimes(1)

				// Get child boost (not found, so create)
				childName := "global-" + name
				mockClient.EXPECT().Get(gomock.Any(),
					gomock.Eq(types.NamespacedName{Name: childName, Namespace: "ns-a"}),
					gomock.AssignableToTypeOf(&autoscaling.StartupCPUBoost{})).
					Return(apierrors.NewNotFound(schema.GroupResource{}, childName))

				// Create the child boost
				mockClient.EXPECT().Create(gomock.Any(), gomock.Cond(func(obj any) bool {
					boost := obj.(*autoscaling.StartupCPUBoost)
					return boost.Name == childName &&
						boost.Namespace == "ns-a" &&
						boost.Labels[autoscaling.ManagedByGlobalBoostLabel] == name
				})).Return(nil)

				// Status update
				mockClient.EXPECT().Status().Return(mockSubResClient)
				mockSubResClient.EXPECT().Update(gomock.Any(), gomock.Cond(func(obj any) bool {
					gb := obj.(*autoscaling.GlobalStartupCPUBoost)
					return gb.Status.NamespaceCount == 1 &&
						len(gb.Status.ActiveNamespaces) == 1 &&
						gb.Status.ActiveNamespaces[0] == "ns-a"
				})).Return(nil)
			})

			It("creates child boosts and updates status", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})

		When("namespace has user-created boost", func() {
			var mockSubResClient *mock.MockSubResourceClient
			BeforeEach(func() {
				mockSubResClient = mock.NewMockSubResourceClient(mockCtrl)
				mockClient.EXPECT().Get(gomock.Any(), gomock.Eq(req.NamespacedName), gomock.Any()).
					DoAndReturn(func(c context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						gb := obj.(*autoscaling.GlobalStartupCPUBoost)
						gb.Name = name
						gb.UID = "test-uid"
						gb.Finalizers = []string{autoscaling.GlobalBoostFinalizer}
						gb.Spec.NamespaceSelector = metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "test"},
						}
						gb.Spec.Template = autoscaling.GlobalStartupCPUBoostTemplate{
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "demo"},
							},
							Spec: autoscaling.StartupCPUBoostSpec{
								DurationPolicy: autoscaling.DurationPolicy{
									Fixed: &autoscaling.FixedDurationPolicy{
										Value: 60,
										Unit:  autoscaling.FixedDurationPolicyUnitSec,
									},
								},
							},
						}
						return nil
					})

				// List namespaces
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NamespaceList{}), gomock.Any()).
					DoAndReturn(func(c context.Context, list client.ObjectList, opts ...client.ListOption) error {
						nsList := list.(*corev1.NamespaceList)
						nsList.Items = []corev1.Namespace{
							{ObjectMeta: metav1.ObjectMeta{Name: "ns-skip", Labels: map[string]string{"env": "test"}}},
						}
						return nil
					})

				// Listing unmanaged boosts - namespace has user-created boost
				listCallCount := 0
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&autoscaling.StartupCPUBoostList{}),
					gomock.Any()).
					DoAndReturn(func(c context.Context, list client.ObjectList, opts ...client.ListOption) error {
						listCallCount++
						boostList := list.(*autoscaling.StartupCPUBoostList)
						if listCallCount == 1 {
							// First call: checking unmanaged boosts in ns-skip
							boostList.Items = []autoscaling.StartupCPUBoost{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "user-boost",
										Namespace: "ns-skip",
									},
								},
							}
						}
						return nil
					}).MinTimes(1)

				// Try to delete existing child (not found)
				childName := "global-" + name
				mockClient.EXPECT().Get(gomock.Any(),
					gomock.Eq(types.NamespacedName{Name: childName, Namespace: "ns-skip"}),
					gomock.AssignableToTypeOf(&autoscaling.StartupCPUBoost{})).
					Return(apierrors.NewNotFound(schema.GroupResource{}, childName)).MaxTimes(1)

				// Status update - no active, one skipped
				mockClient.EXPECT().Status().Return(mockSubResClient)
				mockSubResClient.EXPECT().Update(gomock.Any(), gomock.Cond(func(obj any) bool {
					gb := obj.(*autoscaling.GlobalStartupCPUBoost)
					return gb.Status.NamespaceCount == 0 &&
						len(gb.Status.SkippedNamespaces) == 1 &&
						gb.Status.SkippedNamespaces[0] == "ns-skip"
				})).Return(nil)
			})

			It("skips the namespace and records it in status", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))
			})
		})
	})
})
