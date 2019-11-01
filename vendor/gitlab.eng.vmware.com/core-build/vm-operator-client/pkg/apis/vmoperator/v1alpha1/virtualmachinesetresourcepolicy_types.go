// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineSetResourcePolicy
// +k8s:openapi-gen=true
type VirtualMachineSetResourcePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSetResourcePolicySpec   `json:"spec,omitempty"`
	Status VirtualMachineSetResourcePolicyStatus `json:"status,omitempty"`
}

func (res VirtualMachineSetResourcePolicy) NamespacedName() string {
	return res.Namespace + "/" + res.Name
}

// ResourcePoolSpec defines a Resource Group
type ResourcePoolSpec struct {
	Name         string                          `json:"name,omitempty"`
	Reservations VirtualMachineClassResourceSpec `json:"reservations,omitempty"`
	Limits       VirtualMachineClassResourceSpec `json:"limits,omitempty"`
}

// Folder defines a Folder
type FolderSpec struct {
	Name string `json:"name,omitempty"`
}

// VirtualMachineSetResourcePolicySpec defines the desired state of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicySpec struct {
	ResourcePool ResourcePoolSpec `json:"resourcepool,omitempty"`
	Folder       FolderSpec       `json:"folder,omitempty"`
}

// VirtualMachineSetResourcePolicyStatus defines the observed state of VirtualMachineSetResourcePolicy
type VirtualMachineSetResourcePolicyStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineSetResourcePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineSetResourcePolicy `json:"items"`
}
