/*
Copyright 2019 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/vsphere-csi-driver/pkg/syncer/cnsoperator/apis"
)


// CnsVolumeMetadataSpec defines the desired state of CnsVolumeMetadata
// +k8s:openapi-gen=true
type CnsVolumeMetadataSpec struct {
	// VolumeName indicates the unique ID of the volume.
	// For volumes created in a guest cluster, this will be
	// “<guestcluster-ID>-<UID>” where UID is the pvc.UUID in SVC
	VolumeName string `json:"volumename"`

	// GuestClusterID indicates the guest cluster ID in which this volume
	// is referenced.
	GuestClusterID string `json:"guestclusterid"`

	// EntityType indicates type of entity whose metadata
	// this instance represents.
	// Allowed types are PERSISTENT_VOLUME,
	// PERSISTENT_VOLUME_CLAIM or POD.
	EntityType string `json:"entitytype"`

	// EntityName indicates name of the entity in the guest cluster.
	EntityName string `json:"entityname"`

	// Labels indicates user labels assigned to the entity
	// in the guest cluster. Should only be populated if
	// EntityType is PERSISTENT_VOLUME OR PERSISTENT_VOLUME_CLAIM
	// CNS Operator will return a failure to the client if labels
	// are set for objects whose EntityType is POD.
	//+optional
	Labels map[string]string `json:"labels,omitempty"`

	// Namespace indicates namespace of entity in guest cluster.
	// Should only be populated if EntityType is
	// PERSISTENT_VOLUME_CLAIM or POD.
	// CNS Operator will return a failure to the client if
	// namespace is set for objects whose EntityType is PERSISTENT_VOLUME.
	//+optional
	Namespace string `json:"namespace,omitempty"`

}

// CnsVolumeMetadataStatus defines the observed state of CnsVolumeMetadata
// +k8s:openapi-gen=true
type CnsVolumeMetadataStatus struct {
	// The last error encountered during update operation, if any.
	// This field must only be set by the entity completing the update
	// operation, i.e. the CNS Operator.
	// This string may be logged, so it should not contain sensitive
	// information.
	// +optional
	ErrorMessage string `json:"errormessage,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsVolumeMetadata is the Schema for the cnsvolumemetadata API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=cnsvolumemetadata,scope=Namespaced
type CnsVolumeMetadata struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CnsVolumeMetadataSpec   `json:"spec,omitempty"`
	Status CnsVolumeMetadataStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CnsVolumeMetadataList contains a list of CnsVolumeMetadata
type CnsVolumeMetadataList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CnsVolumeMetadata `json:"items"`
}

// Allowed EntityTypes for CnsVolumeMetadataSpec
const (
	CnsOperatorEntityTypePVC = string("PERSISTENT_VOLUME_CLAIM")
	CnsOperatorEntityTypePV  = string("PERSISTENT_VOLUME")
	CnsOperatorEntityTypePOD = string("POD")
)

func init() {
	apis.SchemeBuilder.Register(&CnsVolumeMetadata{}, &CnsVolumeMetadataList{})
}