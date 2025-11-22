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

	ign3types "github.com/coreos/ignition/v2/config/v3_2/types"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nodeswapv1alpha1 "github.com/openshift-virtualization/swap-operator/api/v1alpha1"
)

// NodeSwapReconciler reconciles a NodeSwap object
type NodeSwapReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=node-swap.openshift.io,resources=nodeswaps/finalizers,verbs=update

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

	// TODO(user): your logic here
	//RednerMachineConfigFromAPI
	//ReconcileRenderedMachineConfigWithCurrentMachineConfig
	//UpdateMachineConfig
	//UpdateNodeSwapStatus
	//Requeue

	return ctrl.Result{}, nil
}

// GenerateSwapMachineConfig creates a MachineConfig object for enabling swap on worker nodes
func GenerateSwapMachineConfig(name, role string, swapSizeMB int) *mcfgv1.MachineConfig {
	// Base64 encoded kubelet configuration for swap
	// Content: apiVersion: kubelet.config.k8s.io/v1beta1\nkind: KubeletConfiguration\nmemorySwap:\n  swapBehavior: LimitedSwap\n
	kubeletConfigSource := "data:text/plain;charset=utf-8;base64,YXBpVmVyc2lvbjoga3ViZWxldC5jb25maWcuazhzLmlvL3YxYmV0YTEKa2luZDogS3ViZWxldENvbmZpZ3VyYXRpb24KbWVtb3J5U3dhcDoKICBzd2FwQmVoYXZpb3I6IExpbWl0ZWRTd2FwCg=="

	// Swap provision service unit content
	swapProvisionUnit := `[Unit]
Description=Provision and enable swap
ConditionFirstBoot=no
ConditionPathExists=!/var/tmp/swapfile

[Service]
Type=oneshot
Environment=SWAP_SIZE_MB=` + string(rune(swapSizeMB)) + `
ExecStart=/bin/sh -c "sudo fallocate -l ${SWAP_SIZE_MB}M /var/tmp/swapfile && \
sudo chmod 600 /var/tmp/swapfile && \
sudo mkswap /var/tmp/swapfile && \
sudo swapon /var/tmp/swapfile && \
free -h"

[Install]
RequiredBy=kubelet-dependencies.target
`

	// System slice cgroup configuration service unit content
	cgroupSystemSliceUnit := `[Unit]
Description=Restrict swap for system slice
ConditionFirstBoot=no

[Service]
Type=oneshot
ExecStart=/bin/sh -c "sudo systemctl set-property --runtime system.slice MemorySwapMax=0 IODeviceLatencyTargetSec=\"/ 50ms\""

[Install]
RequiredBy=kubelet-dependencies.target
`

	// Create the file mode (420 in decimal = 0644 in octal)
	fileMode := 420

	// Create the MachineConfig object
	mc := &mcfgv1.MachineConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "machineconfiguration.openshift.io/v1",
			Kind:       "MachineConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"machineconfiguration.openshift.io/role": role,
			},
		},
		Spec: mcfgv1.MachineConfigSpec{
			Config: ign3types.Config{
				Ignition: ign3types.Ignition{
					Version: "3.2.0",
				},
				Storage: ign3types.Storage{
					Files: []ign3types.File{
						{
							Node: ign3types.Node{
								Path:      "/etc/openshift/kubelet.conf.d/90-swap.conf",
								Overwrite: boolPtr(true),
							},
							FileEmbedded1: ign3types.FileEmbedded1{
								Mode: &fileMode,
								Contents: ign3types.Resource{
									Source: &kubeletConfigSource,
								},
							},
						},
					},
				},
				Systemd: ign3types.Systemd{
					Units: []ign3types.Unit{
						{
							Name:     "swap-provision.service",
							Enabled:  boolPtr(true),
							Contents: &swapProvisionUnit,
						},
						{
							Name:     "cgroup-system-slice-config.service",
							Enabled:  boolPtr(true),
							Contents: &cgroupSystemSliceUnit,
						},
					},
				},
			},
		},
	}

	return mc
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeSwapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodeswapv1alpha1.NodeSwap{}).
		Named("nodeswap").
		Complete(r)
}
