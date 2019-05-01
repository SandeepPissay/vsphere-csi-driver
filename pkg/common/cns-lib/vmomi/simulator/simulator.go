/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.

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

package simulator

import (
	"context"
	"github.com/google/uuid"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/methods"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
)

func New() *simulator.Registry {
	r := simulator.NewRegistry()
	r.Namespace = cnsvsphere.Namespace
	r.Path = cnsvsphere.Path

	r.Put(&CnsVolumeManager{
		ManagedObjectReference: cnsvsphere.CnsVolumeManagerInstance,
		volumes:                make(map[vim25types.ManagedObjectReference]map[cnstypes.CnsVolumeId]*cnstypes.CnsVolume),
	})

	return r
}

type CnsVolumeManager struct {
	vim25types.ManagedObjectReference

	volumes map[vim25types.ManagedObjectReference]map[cnstypes.CnsVolumeId]*cnstypes.CnsVolume
}

func (m *CnsVolumeManager) CnsCreateVolume(ctx context.Context, req *cnstypes.CnsCreateVolume) soap.HasFault {
	task := simulator.CreateTask(m, "CnsCreateVolume", func(*simulator.Task) (vim25types.AnyType, vim25types.BaseMethodFault) {
		if len(req.CreateSpecs) == 0 {
			return nil, &vim25types.InvalidArgument{InvalidProperty: "CnsVolumeCreateSpec"}
		}

		operationResult := []cnstypes.BaseCnsVolumeOperationResult{}
		for _, createSpec := range req.CreateSpecs {
			for _, datastoreRef := range createSpec.Datastores {
				datastore := simulator.Map.Get(datastoreRef).(*simulator.Datastore)

				volumes, ok := m.volumes[datastore.Self]
				if !ok {
					volumes = make(map[cnstypes.CnsVolumeId]*cnstypes.CnsVolume)
					m.volumes[datastore.Self] = volumes
				}

				newVolume := &cnstypes.CnsVolume{
					VolumeId: cnstypes.CnsVolumeId{
						Id: uuid.New().String(),
					},
					Name:                         createSpec.Name,
					VolumeType:                   createSpec.VolumeType,
					DatastoreUrl:                 datastore.Info.GetDatastoreInfo().Url,
					Metadata:                     createSpec.Metadata,
					BackingObjectDetails:         *createSpec.BackingObjectDetails.GetCnsBackingObjectDetails(),
					ComplianceStatus:             "Simulator Compliance Status",
					DatastoreAccessibilityStatus: "Simulator Datastore Accessibility Status",
					StoragePolicyId:              "Simulator Storage Policy ID",
				}

				volumes[newVolume.VolumeId] = newVolume
				operationResult = append(operationResult, &cnstypes.CnsVolumeOperationResult{
					VolumeId: newVolume.VolumeId,
				})
			}
		}

		return &cnstypes.CnsVolumeOperationBatchResult{
			VolumeResults: operationResult,
		}, nil
	})

	return &methods.CnsCreateVolumeBody{
		Res: &cnstypes.CnsCreateVolumeResponse{
			Returnval: task.Run(),
		},
	}
}

func (m *CnsVolumeManager) CnsQueryVolume(ctx context.Context, req *cnstypes.CnsQueryVolume) soap.HasFault {
	retVolumes := []cnstypes.CnsVolume{}
	for _, dsVolumes := range m.volumes {
		for _, volume := range dsVolumes {
			retVolumes = append(retVolumes, *volume)
		}
	}

	return &methods.CnsQueryVolumeBody{
		Res: &cnstypes.CnsQueryVolumeResponse{
			Returnval: cnstypes.CnsQueryResult{
				Volumes: retVolumes,
				Cursor:  cnstypes.CnsCursor{},
			},
		},
	}
}

func (m *CnsVolumeManager) CnsDeleteVolume(ctx context.Context, req *cnstypes.CnsDeleteVolume) soap.HasFault {
	task := simulator.CreateTask(m, "CnsDeleteVolume", func(*simulator.Task) (vim25types.AnyType, vim25types.BaseMethodFault) {
		operationResult := []cnstypes.BaseCnsVolumeOperationResult{}
		for _, volumeId := range req.VolumeIds {
			for ds, dsVolumes := range m.volumes {
				volume := dsVolumes[volumeId]
				if volume != nil {
					delete(m.volumes[ds], volumeId)
					operationResult = append(operationResult, &cnstypes.CnsVolumeOperationResult{
						VolumeId: volumeId,
					})

				}
			}
		}
		return &cnstypes.CnsVolumeOperationBatchResult{
			VolumeResults: operationResult,
		}, nil
	})

	return &methods.CnsDeleteVolumeBody{
		Res: &cnstypes.CnsDeleteVolumeResponse{
			Returnval: task.Run(),
		},
	}
}
