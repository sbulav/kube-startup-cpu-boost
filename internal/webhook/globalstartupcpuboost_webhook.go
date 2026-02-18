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

package webhook

import (
	"context"

	"github.com/google/kube-startup-cpu-boost/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type GlobalStartupCPUBoostWebhook struct{}

var _ webhook.CustomValidator = &GlobalStartupCPUBoostWebhook{}

func setupWebhookForGlobalStartupCPUBoost(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.GlobalStartupCPUBoost{}).
		WithValidator(&GlobalStartupCPUBoostWebhook{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-autoscaling-x-k8s-io-v1alpha1-globalstartupcpuboost,mutating=false,failurePolicy=fail,sideEffects=None,groups=autoscaling.x-k8s.io,resources=globalstartupcpuboosts,verbs=create;update,versions=v1alpha1,name=vglobalstartupcpuboost.autoscaling.x-k8s.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.CustomValidator
func (w *GlobalStartupCPUBoostWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	boost := obj.(*v1alpha1.GlobalStartupCPUBoost)
	log := ctrl.LoggerFrom(ctx).WithName("global-boost-validate-webhook")
	log.V(5).Info("handling create validation", "globalstartupcpuboost", klog.KObj(boost))
	return nil, validateGlobal(boost)
}

// ValidateUpdate implements webhook.CustomValidator
func (w *GlobalStartupCPUBoostWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	boost := newObj.(*v1alpha1.GlobalStartupCPUBoost)
	log := ctrl.LoggerFrom(ctx).WithName("global-boost-validate-webhook")
	log.V(5).Info("handling update validation", "globalstartupcpuboost", klog.KObj(boost))
	return nil, validateGlobal(boost)
}

// ValidateDelete implements webhook.CustomValidator
func (w *GlobalStartupCPUBoostWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateGlobal verifies if GlobalStartupCPUBoost is valid
func validateGlobal(boost *v1alpha1.GlobalStartupCPUBoost) error {
	var allErrs field.ErrorList

	// Validate namespace selector (including MatchLabels and MatchExpressions)
	nsFldPath := field.NewPath("spec").Child("namespaceSelector")
	if _, err := metav1.LabelSelectorAsSelector(&boost.Spec.NamespaceSelector); err != nil {
		allErrs = append(allErrs, field.Invalid(nsFldPath, boost.Spec.NamespaceSelector, err.Error()))
	}

	// Validate template pod selector (including MatchLabels and MatchExpressions)
	selectorFldPath := field.NewPath("spec").Child("template").Child("selector")
	if _, err := metav1.LabelSelectorAsSelector(&boost.Spec.Template.Selector); err != nil {
		allErrs = append(allErrs, field.Invalid(selectorFldPath, boost.Spec.Template.Selector, err.Error()))
	}

	// Validate container policies (reuse existing validation)
	if errs := validateContainerPolicies(boost.Spec.Template.Spec.ResourcePolicy.ContainerPolicies); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate duration policy (reuse existing validation)
	if err := validateDurationPolicy(boost.Spec.Template.Spec.DurationPolicy); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "autoscaling.x-k8s.io", Kind: "GlobalStartupCPUBoost"},
			boost.Name, allErrs)
	}
	return nil
}
