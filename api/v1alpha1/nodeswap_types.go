/*
Copyright The Swap Operator authors.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubeSwapMode string

const (
	KubeSwapLimited   KubeSwapMode = "Limited"
	KubeSwapUnlimited KubeSwapMode = "Unlimited"
)

// KubeletConfig defines swap related configuration for kubelet
type KubeletConfig struct {
	// +kubebuilder:validation:Type=string
	SwapMode KubeSwapMode `json:"swapMode,omitempty"`
}

type SwapType string

const (
	FileBasedSwap SwapType = "file"
	SwapOnZram    SwapType = "zram"
	SwapOnDisk    SwapType = "disk"
)

type SwapFile struct {
	Path string            `json:"path,omitempty"`
	Size resource.Quantity `json:"size,omitempty"`
}

type Partition struct {
	PartLabel string `json:"partlabel,omitempty"`
}

type SwapZram struct {
	Size resource.Quantity `json:"size,omitempty"`
}

type SwapDisk struct {
	SwapPartition Partition `json:"partition,omitempty"`
}

type SwapSpec struct {
	Priority int32 `json:"priority,omitempty"`

	SwapType SwapType `json:"swapType,omitempty"`

	// +optional
	Disk *SwapDisk `json:"disk,omitempty"`

	// +optional
	File *SwapFile `json:"file,omitempty"`

	// +optional
	Zram *SwapZram `json:"zram,omitempty"`
}

type Swaps []SwapSpec

// NodeSwapSpec defines the desired state of NodeSwap
type NodeSwapSpec struct {
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Label selector for Machines on which swap will be deployed.
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	Swaps Swaps `json:"swaps,omitempty"`

	// +optional
	KubeletConfig *KubeletConfig `json:"kubeletConfig,omitempty"`

	// +optional
	LogLevel *int32 `json:"logLevel,omitempty"`
}

// NodeSwapStatus defines the observed state of NodeSwap.
type NodeSwapStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the NodeSwap resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NodeSwap is the Schema for the nodeswaps API
type NodeSwap struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// +required
	Spec NodeSwapSpec `json:"spec"`

	// +optional
	Status NodeSwapStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// NodeSwapList contains a list of NodeSwap
type NodeSwapList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSwap `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeSwap{}, &NodeSwapList{})
}
