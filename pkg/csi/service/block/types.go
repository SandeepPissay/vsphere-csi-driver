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

package block

import (
	cnsvolume "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
)

// Manager type comprises VirtualCenterConfig, CnsConfig, VolumeManager and VirtualCenterManager
type Manager struct {
	VcenterConfig  *cnsvsphere.VirtualCenterConfig
	CnsConfig      *config.Config
	VolumeManager  cnsvolume.Manager
	VcenterManager cnsvsphere.VirtualCenterManager
}

// CreateVolumeSpec is the Volume Spec used by CSI driver
type CreateVolumeSpec struct {
	Name              string
	StoragePolicyName string
	StoragePolicyID   string
	Datastore         string
	CapacityMB        int64
}
