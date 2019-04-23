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
	cspvolume "gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume"
	cnsvsphere "gitlab.eng.vmware.com/hatchway/common-csp/pkg/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
)

type Manager struct {
	VcenterConfig  *cnsvsphere.VirtualCenterConfig
	CnsConfig      *config.Config
	VolumeManager  cspvolume.Manager
	VcenterManager cnsvsphere.VirtualCenterManager
}

type CreateVolumeSpec struct {
	Name              string
	StoragePolicyName string
	StoragePolicyID   string
	Datastore         string
	CapacityMB        uint64
}
