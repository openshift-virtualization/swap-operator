/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	"go.yaml.in/yaml/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nodeswap "github.com/openshift-virtualization/swap-operator/api/v1alpha1"
	"github.com/openshift-virtualization/swap-operator/internal/renderconfig"
	"github.com/openshift-virtualization/swap-operator/internal/template"
)

const (
	typeAvailableNodeSwap   = "Availabe"
	typeProgressingNodeSwap = "Progressing"
	typeDegradedNodeSwap    = "Degraded"
)

type NodeSwapReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	TemplateDir     string
	config          []renderconfig.RenderConfig
	ctx             context.Context
	desiredNodeSwap nodeswap.NodeSwap
	mcpReady        bool
}

// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps/finalizers,verbs=update
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigpools,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeSwap object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *NodeSwapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)
	r.ctx = ctx

	if err := r.Get(ctx, req.NamespacedName, &r.desiredNodeSwap); err != nil {
		if apierrors.IsNotFound(err) {
			logf.FromContext(ctx).Info("NodeSwap resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logf.FromContext(ctx).Error(err, "Failed to get NodeSwap")
		return ctrl.Result{}, err
	}

	// Reconcile the spec and capture any errors
	_, reconcileErr := r.ReconcileSpec()

	// Always update status with the result (success or failure)
	if _, statusErr := r.ReconcileStatus(reconcileErr); statusErr != nil {
		logf.FromContext(ctx).Error(statusErr, "Failed to update status")
		// Return both errors if status update fails
		if reconcileErr != nil {
			return ctrl.Result{}, fmt.Errorf("reconcile error: %v, status update error: %v", reconcileErr, statusErr)
		}
		return ctrl.Result{}, statusErr
	}

	// Return the original reconcile error (status was updated successfully)
	return ctrl.Result{}, reconcileErr
}

func (r *NodeSwapReconciler) ReconcileStatus(reconcileErr error) (ctrl.Result, error) {
	if reconcileErr != nil {
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeDegradedNodeSwap,
			Status:  metav1.ConditionTrue,
			Reason:  "ReconciliationFailed",
			Message: fmt.Sprintf("Failed to reconcile: %v", reconcileErr),
		})
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeProgressingNodeSwap,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationFailed",
			Message: "Reconciliation failed",
		})
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeAvailableNodeSwap,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationFailed",
			Message: "",
		})
	} else if len(r.desiredNodeSwap.Status.Conditions) == 0 {
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeProgressingNodeSwap,
			Status:  metav1.ConditionTrue,
			Reason:  "Reconciling",
			Message: "NodeSwap is reconciling",
		})
	} else {
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeAvailableNodeSwap,
			Status:  metav1.ConditionTrue,
			Reason:  "ReconciliationSucceeded",
			Message: "NodeSwap successfully reconciled",
		})
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeDegradedNodeSwap,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationSucceeded",
			Message: "",
		})
		meta.SetStatusCondition(&r.desiredNodeSwap.Status.Conditions, metav1.Condition{
			Type:    typeProgressingNodeSwap,
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationSucceeded",
			Message: "",
		})
	}

	if err := r.Status().Update(r.ctx, &r.desiredNodeSwap); err != nil {
		logf.FromContext(r.ctx).Error(err, "Failed to update NodeSwap status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeSwapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeswap.NodeSwap{}).
		Watches(&mcfgv1.MachineConfigPool{}, &handler.EnqueueRequestForObject{}).
		Owns(&mcfgv1.MachineConfig{}).
		Named("nodeswap").
		Complete(r)
}

func (r *NodeSwapReconciler) ReconcileKubeletCgroups() (ctrl.Result, error) {
	var kubeletMachineConfig mcfgv1.MachineConfig
	if err := r.Get(r.ctx,
		types.NamespacedName{Name: renderconfig.SwapKubeletCgroupsMCPrefix},
		&kubeletMachineConfig); err != nil {
		if apierrors.IsNotFound(err) {
			fullTemplatePath := filepath.Join(r.TemplateDir, "worker", "99-swap-kubelet-cgroups")
			mc, err := template.GenerateMachineConfigForName(
				&renderconfig.RenderConfig{},
				"worker",
				"99-swap-kubelet-cgroups",
				r.TemplateDir,
				fullTemplatePath,
			)
			if err != nil {
				logf.FromContext(r.ctx).Error(err, "Failed to render kubelet machine config")
				return ctrl.Result{}, err
			}

			mcBytes, err := yaml.Marshal(mc)
			if err != nil {
				logf.FromContext(r.ctx).Error(err, "Failed to marshal MachineConfig")
			} else {
				mcBase64 := base64.StdEncoding.EncodeToString(mcBytes)
				logf.FromContext(r.ctx).Info("Generated MachineConfig", "base64", mcBase64)
			}

			if mc.ObjectMeta.Labels == nil {
				mc.ObjectMeta.Labels = map[string]string{}
			}

			key, value, err := parseLabelSelector(r.desiredNodeSwap.Spec.MachineConfigPoolSelector)
			if err != nil {
				logf.FromContext(r.ctx).Error(err, "Failed to parse label selector")
				return ctrl.Result{}, err
			}
			mc.ObjectMeta.Labels[key] = value

			if err := r.Create(r.ctx, mc); errors.IsAlreadyExists(err) {
				logf.FromContext(r.ctx).Info("Kubelet machine config already exists")
				return ctrl.Result{}, nil
			} else if err != nil {
				logf.FromContext(r.ctx).Error(err, "Failed to create kubelet machine config")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
		logf.FromContext(r.ctx).Error(err, "Failed to get kubelet machine config")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeSwapReconciler) ReconcileSpec() (ctrl.Result, error) {
	config, err := renderconfig.Create(&r.desiredNodeSwap.Spec)
	if err != nil {
		logf.FromContext(r.ctx).Error(err, "Failed to create render config")
		return ctrl.Result{}, err
	}
	r.config = config

	// List all MachineConfigPools
	mcpList := &mcfgv1.MachineConfigPoolList{}
	if err := r.List(r.ctx, mcpList); err != nil {
		logf.FromContext(r.ctx).Error(err, "Failed to list MachineConfigPools")
		return ctrl.Result{}, err
	}

	// Parse the desired label selector
	labelKey, labelValue, err := parseLabelSelector(r.desiredNodeSwap.Spec.MachineConfigPoolSelector)
	if err != nil {
		logf.FromContext(r.ctx).Error(err, "Failed to parse label selector")
		return ctrl.Result{}, err
	}

	// Filter MachineConfigPools that match the selector
	var matchingMCPs []*mcfgv1.MachineConfigPool
	var updatedMCPs, notUpdatedMCPs []*mcfgv1.MachineConfigPool

	for i := range mcpList.Items {
		mcp := &mcpList.Items[i]
		if mcp.Spec.MachineConfigSelector != nil &&
			mcp.Spec.MachineConfigSelector.MatchLabels != nil {
			if value, exists :=
				mcp.Spec.MachineConfigSelector.MatchLabels[labelKey]; exists && value == labelValue {
				matchingMCPs = append(matchingMCPs, mcp)

				if isMachineConfigPoolUpdated(mcp) {
					updatedMCPs = append(updatedMCPs, mcp)
					logf.FromContext(r.ctx).Info("MachineConfigPool is updated",
						"name", mcp.Name)
				} else {
					notUpdatedMCPs = append(notUpdatedMCPs, mcp)
					logf.FromContext(r.ctx).Info("MachineConfigPool is not yet updated",
						"name", mcp.Name)
				}
			}
		}
	}

	logf.FromContext(r.ctx).Info("MachineConfigPool filtering complete",
		"matchingCount", len(matchingMCPs),
		"updatedCount", len(updatedMCPs),
		"notUpdatedCount", len(notUpdatedMCPs))

	r.mcpReady = len(notUpdatedMCPs) == 0

	return r.ReconcileKubeletCgroups()
}

// parseLabelSelector parses a label selector string in the format "key:" or "key:value"
// and returns the key and value separately. The key is required but the value can be empty.
func parseLabelSelector(selector string) (string, string, error) {
	if selector == "" {
		return "", "", fmt.Errorf("label selector is empty")
	}

	parts := strings.SplitN(selector, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid label selector format: %s, expected key: or key:value", selector)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		return "", "", fmt.Errorf("label selector has empty key: %s", selector)
	}

	return key, value, nil
}

// isMachineConfigPoolUpdated checks if a MachineConfigPool is fully updated.
// Returns true if the pool's Updated condition is True, and both Updating and Degraded are False.
func isMachineConfigPoolUpdated(mcp *mcfgv1.MachineConfigPool) bool {
	var updated, updating, degraded bool

	for _, condition := range mcp.Status.Conditions {
		switch condition.Type {
		case mcfgv1.MachineConfigPoolUpdated:
			updated = condition.Status == corev1.ConditionTrue
		case mcfgv1.MachineConfigPoolUpdating:
			updating = condition.Status == corev1.ConditionTrue
		case mcfgv1.MachineConfigPoolDegraded:
			degraded = condition.Status == corev1.ConditionTrue
		}
	}

	// Pool is considered updated if:
	// - Updated condition is True
	// - Updating condition is False (or not found)
	// - Degraded condition is False (or not found)
	return updated && !updating && !degraded
}
