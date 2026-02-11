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

package controller

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	autoscaling "github.com/google/kube-startup-cpu-boost/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	GlobalBoostActiveConditionTrueReason   = "Ready"
	GlobalBoostActiveConditionTrueMessage  = "Distributing StartupCPUBoost to matching namespaces"
	GlobalBoostActiveConditionFalseReason  = "NotReady"
	GlobalBoostActiveConditionFalseMessage = "GlobalStartupCPUBoost is not active"
)

// GlobalStartupCPUBoostReconciler reconciles a GlobalStartupCPUBoost object
type GlobalStartupCPUBoostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=autoscaling.x-k8s.io,resources=globalstartupcpuboosts,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=autoscaling.x-k8s.io,resources=globalstartupcpuboosts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoscaling.x-k8s.io,resources=globalstartupcpuboosts/finalizers,verbs=update
//+kubebuilder:rbac:groups=autoscaling.x-k8s.io,resources=startupcpuboosts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile handles GlobalStartupCPUBoost events.
func (r *GlobalStartupCPUBoostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("name", req.Name)

	var globalBoost autoscaling.GlobalStartupCPUBoost
	if err := r.Client.Get(ctx, req.NamespacedName, &globalBoost); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion with finalizer
	if !globalBoost.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &globalBoost, log)
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(&globalBoost, autoscaling.GlobalBoostFinalizer) {
		controllerutil.AddFinalizer(&globalBoost, autoscaling.GlobalBoostFinalizer)
		if err := r.Client.Update(ctx, &globalBoost); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return r.reconcile(ctx, &globalBoost, log)
}

// reconcile performs the main reconciliation logic.
func (r *GlobalStartupCPUBoostReconciler) reconcile(ctx context.Context,
	globalBoost *autoscaling.GlobalStartupCPUBoost, log logr.Logger) (ctrl.Result, error) {

	// List namespaces matching the selector
	nsSelector, err := metav1.LabelSelectorAsSelector(&globalBoost.Spec.NamespaceSelector)
	if err != nil {
		log.Error(err, "failed to parse namespace selector")
		return ctrl.Result{}, err
	}

	var namespaceList corev1.NamespaceList
	if err := r.Client.List(ctx, &namespaceList, &client.ListOptions{
		LabelSelector: nsSelector,
	}); err != nil {
		log.Error(err, "failed to list namespaces")
		return ctrl.Result{}, err
	}

	childName := childBoostName(globalBoost.Name)
	activeNamespaces := make([]string, 0)
	skippedNamespaces := make([]string, 0)

	// Track which namespaces should have children
	desiredNamespaces := make(map[string]bool)
	for _, ns := range namespaceList.Items {
		if ns.DeletionTimestamp != nil {
			continue
		}
		desiredNamespaces[ns.Name] = true
	}

	// Process each matching namespace
	for nsName := range desiredNamespaces {
		log := log.WithValues("namespace", nsName)

		// Check for user-created (unmanaged) boosts in this namespace
		hasUnmanaged, err := r.hasUnmanagedBoosts(ctx, nsName, globalBoost.Name)
		if err != nil {
			log.Error(err, "failed to check for unmanaged boosts")
			continue
		}
		if hasUnmanaged {
			log.V(5).Info("skipping namespace with user-created boosts")
			skippedNamespaces = append(skippedNamespaces, nsName)
			// Clean up any previously created child in this namespace
			r.deleteChildIfExists(ctx, childName, nsName, log)
			continue
		}

		// Create or update child StartupCPUBoost
		if err := r.ensureChild(ctx, globalBoost, nsName, childName, log); err != nil {
			log.Error(err, "failed to ensure child boost")
			continue
		}
		activeNamespaces = append(activeNamespaces, nsName)
	}

	// Delete children in namespaces that no longer match
	if err := r.cleanupStaleChildren(ctx, globalBoost.Name, childName, desiredNamespaces, activeNamespaces, log); err != nil {
		log.Error(err, "failed to cleanup stale children")
	}

	// Update status
	sort.Strings(activeNamespaces)
	sort.Strings(skippedNamespaces)
	return r.updateStatus(ctx, globalBoost, activeNamespaces, skippedNamespaces, log)
}

// handleDeletion removes all managed children and the finalizer.
func (r *GlobalStartupCPUBoostReconciler) handleDeletion(ctx context.Context,
	globalBoost *autoscaling.GlobalStartupCPUBoost, log logr.Logger) (ctrl.Result, error) {

	if !controllerutil.ContainsFinalizer(globalBoost, autoscaling.GlobalBoostFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("handling deletion, cleaning up child boosts")
	if err := r.deleteAllChildren(ctx, globalBoost.Name, log); err != nil {
		log.Error(err, "failed to delete all children")
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(globalBoost, autoscaling.GlobalBoostFinalizer)
	if err := r.Client.Update(ctx, globalBoost); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

// hasUnmanagedBoosts returns true if the namespace contains any StartupCPUBoost
// that is not managed by the given global boost.
func (r *GlobalStartupCPUBoostReconciler) hasUnmanagedBoosts(ctx context.Context,
	namespace string, globalBoostName string) (bool, error) {
	var boostList autoscaling.StartupCPUBoostList
	if err := r.Client.List(ctx, &boostList, &client.ListOptions{
		Namespace: namespace,
	}); err != nil {
		return false, err
	}
	for _, boost := range boostList.Items {
		if boost.Labels[autoscaling.ManagedByGlobalBoostLabel] != globalBoostName {
			return true, nil
		}
	}
	return false, nil
}

// ensureChild creates or updates a child StartupCPUBoost in the target namespace.
func (r *GlobalStartupCPUBoostReconciler) ensureChild(ctx context.Context,
	globalBoost *autoscaling.GlobalStartupCPUBoost, namespace string, childName string,
	log logr.Logger) error {

	desired := r.buildChildBoost(globalBoost, namespace, childName)

	var existing autoscaling.StartupCPUBoost
	err := r.Client.Get(ctx, types.NamespacedName{Name: childName, Namespace: namespace}, &existing)
	if apierrors.IsNotFound(err) {
		log.V(5).Info("creating child boost", "child", childName)
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create child boost: %w", err)
		}
		log.Info("child boost created", "child", childName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get child boost: %w", err)
	}

	// Verify it's managed by this global boost
	if existing.Labels[autoscaling.ManagedByGlobalBoostLabel] != globalBoost.Name {
		return fmt.Errorf("child boost %s/%s exists but is not managed by this global boost", namespace, childName)
	}

	// Update if spec or selector changed
	if !equality.Semantic.DeepEqual(existing.Selector, desired.Selector) ||
		!equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Selector = desired.Selector
		existing.Spec = desired.Spec
		log.V(5).Info("updating child boost", "child", childName)
		if err := r.Client.Update(ctx, &existing); err != nil {
			return fmt.Errorf("failed to update child boost: %w", err)
		}
		log.Info("child boost updated", "child", childName)
	}
	return nil
}

// buildChildBoost constructs the desired child StartupCPUBoost.
func (r *GlobalStartupCPUBoostReconciler) buildChildBoost(
	globalBoost *autoscaling.GlobalStartupCPUBoost, namespace string,
	childName string) *autoscaling.StartupCPUBoost {

	return &autoscaling.StartupCPUBoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      childName,
			Namespace: namespace,
			Labels: map[string]string{
				autoscaling.ManagedByGlobalBoostLabel: globalBoost.Name,
			},
			Annotations: map[string]string{
				"autoscaling.x-k8s.io/global-boost-uid": string(globalBoost.UID),
			},
		},
		Selector: *globalBoost.Spec.Template.Selector.DeepCopy(),
		Spec:     *globalBoost.Spec.Template.Spec.DeepCopy(),
	}
}

// deleteChildIfExists deletes a child boost if it exists and is managed by this controller.
func (r *GlobalStartupCPUBoostReconciler) deleteChildIfExists(ctx context.Context,
	childName string, namespace string, log logr.Logger) {

	var existing autoscaling.StartupCPUBoost
	err := r.Client.Get(ctx, types.NamespacedName{Name: childName, Namespace: namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		log.Error(err, "failed to get child boost for deletion")
		return
	}
	if _, ok := existing.Labels[autoscaling.ManagedByGlobalBoostLabel]; !ok {
		return
	}
	if err := r.Client.Delete(ctx, &existing); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete child boost", "child", childName, "namespace", namespace)
	} else {
		log.V(5).Info("deleted child boost from skipped namespace", "child", childName, "namespace", namespace)
	}
}

// cleanupStaleChildren removes children from namespaces that no longer match.
func (r *GlobalStartupCPUBoostReconciler) cleanupStaleChildren(ctx context.Context,
	globalBoostName string, childName string, desiredNamespaces map[string]bool,
	activeNamespaces []string, log logr.Logger) error {

	// Build set of namespaces that should have children
	activeSet := make(map[string]bool)
	for _, ns := range activeNamespaces {
		activeSet[ns] = true
	}

	// List all children managed by this global boost
	var allBoosts autoscaling.StartupCPUBoostList
	if err := r.Client.List(ctx, &allBoosts, &client.MatchingLabels{
		autoscaling.ManagedByGlobalBoostLabel: globalBoostName,
	}); err != nil {
		return fmt.Errorf("failed to list managed boosts: %w", err)
	}

	for _, boost := range allBoosts.Items {
		if !activeSet[boost.Namespace] {
			log.V(5).Info("deleting stale child boost", "child", boost.Name, "namespace", boost.Namespace)
			if err := r.Client.Delete(ctx, &boost); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "failed to delete stale child boost", "child", boost.Name, "namespace", boost.Namespace)
			} else {
				log.Info("deleted stale child boost", "child", boost.Name, "namespace", boost.Namespace)
			}
		}
	}
	return nil
}

// deleteAllChildren deletes all children managed by a global boost.
func (r *GlobalStartupCPUBoostReconciler) deleteAllChildren(ctx context.Context,
	globalBoostName string, log logr.Logger) error {

	var allBoosts autoscaling.StartupCPUBoostList
	if err := r.Client.List(ctx, &allBoosts, &client.MatchingLabels{
		autoscaling.ManagedByGlobalBoostLabel: globalBoostName,
	}); err != nil {
		return fmt.Errorf("failed to list managed boosts: %w", err)
	}

	for _, boost := range allBoosts.Items {
		log.V(5).Info("deleting child boost", "child", boost.Name, "namespace", boost.Namespace)
		if err := r.Client.Delete(ctx, &boost); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete child boost %s/%s: %w", boost.Namespace, boost.Name, err)
		}
	}
	return nil
}

// updateStatus updates the GlobalStartupCPUBoost status.
func (r *GlobalStartupCPUBoostReconciler) updateStatus(ctx context.Context,
	globalBoost *autoscaling.GlobalStartupCPUBoost, activeNamespaces []string,
	skippedNamespaces []string, log logr.Logger) (ctrl.Result, error) {

	newStatus := globalBoost.DeepCopy()
	newStatus.Status.NamespaceCount = int32(len(activeNamespaces))
	newStatus.Status.ActiveNamespaces = activeNamespaces
	newStatus.Status.SkippedNamespaces = skippedNamespaces

	activeCondition := metav1.Condition{
		Type:    "Active",
		Status:  metav1.ConditionTrue,
		Reason:  GlobalBoostActiveConditionTrueReason,
		Message: GlobalBoostActiveConditionTrueMessage,
	}
	if len(activeNamespaces) == 0 {
		activeCondition.Status = metav1.ConditionFalse
		activeCondition.Reason = GlobalBoostActiveConditionFalseReason
		activeCondition.Message = GlobalBoostActiveConditionFalseMessage
	}
	meta.SetStatusCondition(&newStatus.Status.Conditions, activeCondition)

	if !equality.Semantic.DeepEqual(newStatus.Status, globalBoost.Status) {
		log.V(5).Info("updating global boost status",
			"activeNamespaces", len(activeNamespaces),
			"skippedNamespaces", len(skippedNamespaces))
		if err := r.Client.Status().Update(ctx, newStatus); err != nil {
			if apierrors.IsConflict(err) {
				log.V(5).Info("status update conflict, requeueing")
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	return ctrl.Result{}, nil
}

// childBoostName returns the name of the child StartupCPUBoost for a given
// GlobalStartupCPUBoost name.
func childBoostName(globalBoostName string) string {
	return autoscaling.GlobalBoostChildPrefix + globalBoostName
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalStartupCPUBoostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscaling.GlobalStartupCPUBoost{}).
		// Watch namespaces to react to new/deleted/relabeled namespaces
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(
			r.mapNamespaceToGlobalBoosts,
		)).
		// Watch StartupCPUBoost to detect user-created boosts affecting skip logic
		Watches(&autoscaling.StartupCPUBoost{}, handler.EnqueueRequestsFromMapFunc(
			r.mapBoostToGlobalBoosts,
		)).
		Complete(r)
}

// mapNamespaceToGlobalBoosts maps namespace events to all GlobalStartupCPUBoost
// reconcile requests, since any namespace change could affect any global boost.
func (r *GlobalStartupCPUBoostReconciler) mapNamespaceToGlobalBoosts(
	ctx context.Context, obj client.Object) []reconcile.Request {

	var globalBoostList autoscaling.GlobalStartupCPUBoostList
	if err := r.Client.List(ctx, &globalBoostList); err != nil {
		r.Log.Error(err, "failed to list global boosts for namespace mapping")
		return nil
	}

	ns := obj.(*corev1.Namespace)
	requests := make([]reconcile.Request, 0)
	for _, gb := range globalBoostList.Items {
		nsSelector, err := metav1.LabelSelectorAsSelector(&gb.Spec.NamespaceSelector)
		if err != nil {
			continue
		}
		if nsSelector.Matches(labels.Set(ns.Labels)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: gb.Name},
			})
		}
	}
	return requests
}

// mapBoostToGlobalBoosts maps StartupCPUBoost events to GlobalStartupCPUBoost
// reconcile requests for global boosts targeting the boost's namespace.
func (r *GlobalStartupCPUBoostReconciler) mapBoostToGlobalBoosts(
	ctx context.Context, obj client.Object) []reconcile.Request {

	boost := obj.(*autoscaling.StartupCPUBoost)

	// Get the namespace object to check its labels
	var ns corev1.Namespace
	if err := r.Client.Get(ctx, types.NamespacedName{Name: boost.Namespace}, &ns); err != nil {
		r.Log.Error(err, "failed to get namespace for boost mapping", "namespace", boost.Namespace)
		return nil
	}

	var globalBoostList autoscaling.GlobalStartupCPUBoostList
	if err := r.Client.List(ctx, &globalBoostList); err != nil {
		r.Log.Error(err, "failed to list global boosts for boost mapping")
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for _, gb := range globalBoostList.Items {
		nsSelector, err := metav1.LabelSelectorAsSelector(&gb.Spec.NamespaceSelector)
		if err != nil {
			continue
		}
		if nsSelector.Matches(labels.Set(ns.Labels)) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: gb.Name},
			})
		}
	}
	return requests
}
