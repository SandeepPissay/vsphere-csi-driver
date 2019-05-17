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
	"errors"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
	"k8s.io/klog"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
)

// CreateVolumeUtil is the helper function to create CNS volume
func CreateVolumeUtil(ctx context.Context, manager *Manager, spec *CreateVolumeSpec, sharedDatastores []*vsphere.DatastoreInfo) (string, error) {
	vc, err := GetVCenter(ctx, manager)
	if err != nil {
		klog.Errorf("Failed to get vCenter from Manager, err: %+v", err)
		return "", err
	}
	if spec.StoragePolicyName != "" {
		// Get Storage Policy ID from Storage Policy Name
		err = vc.ConnectPbm(ctx)
		if err != nil {
			klog.Errorf("Error occurred while connecting to PBM, err: %+v", err)
			return "", err
		}
		spec.StoragePolicyID, err = vc.GetStoragePolicyIDByName(ctx, spec.StoragePolicyName)
		if err != nil {
			klog.Errorf("Error occurred while getting Profile Id from Profile Name: %s, err: %+v", spec.StoragePolicyName, err)
			return "", err
		}
	}
	var datastores []vim25types.ManagedObjectReference
	if spec.Datastore == "" {
		//  If datastore is not specified in storageclass, get all shared datastores
		datastores = getDatastoreMoRefs(sharedDatastores)
	} else {
		// Check datastore specified in the StorageClass should be shared datastore across all nodes.

		// vc.GetDatacenters returns datacenters found on the VirtualCenter.
		// If no datacenters are mentioned in the VirtualCenterConfig during registration, all
		// Datacenters for the given VirtualCenter will be returned, else only the listed
		// Datacenters are returned.
		datacenters, err := vc.GetDatacenters(ctx)
		if err != nil {
			klog.Errorf("Failed to find datacenters from VC: %+v, Error: %+v", vc.Config.Host, err)
			return "", err
		}
		isSharedDatastoreURL := false
		var datastoreObj *vsphere.Datastore
		for _, datacenter := range datacenters {
			datastoreObj, err = datacenter.GetDatastoreByName(ctx, spec.Datastore)
			if err != nil {
				klog.Warningf("Failed to find datastore:%+v in datacenter:%s from VC:%s, Error: %+v", spec.Datastore, datacenter.InventoryPath, vc.Config.Host, err)
			}
			var datastoreURL string
			datastoreURL, err = datastoreObj.GetDatastoreURL(ctx)
			if err != nil {
				klog.Errorf("Failed to get URL for the datastore:%s , Error: %+v", spec.Datastore, err)
				return "", err
			}
			for _, sharedDatastore := range sharedDatastores {
				if sharedDatastore.Info.Url == datastoreURL {
					isSharedDatastoreURL = true
					break
				}
			}
			if isSharedDatastoreURL {
				break
			}
		}
		if datastoreObj == nil {
			errMsg := fmt.Sprintf("Datastore: %s specified in the storage class is not found.", spec.Datastore)
			klog.Errorf(errMsg)
			return "", errors.New(errMsg)
		}
		if isSharedDatastoreURL {
			datastores = append(datastores, datastoreObj.Reference())
		} else {
			errMsg := fmt.Sprintf("Datastore: %s specified in the storage class is not accessible to all nodes.", spec.Datastore)
			klog.Errorf(errMsg)
			return "", errors.New(errMsg)
		}
	}
	createSpec := &cnstypes.CnsVolumeCreateSpec{
		Name:       spec.Name,
		VolumeType: BlockVolumeType,
		Datastores: datastores,
		BackingObjectDetails: &cnstypes.CnsBackingObjectDetails{
			CapacityInMb: spec.CapacityMB,
		},
		Metadata: cnstypes.CnsVolumeMetadata{
			ContainerCluster: vsphere.GetContainerCluster(manager.CnsConfig.Global.ClusterID, manager.CnsConfig.VirtualCenter[vc.Config.Host].User),
		},
	}
	if spec.StoragePolicyID != "" {
		profileSpec := &vim25types.VirtualMachineDefinedProfileSpec{
			ProfileId: spec.StoragePolicyID,
		}
		createSpec.Profile = append(createSpec.Profile, profileSpec)
	}
	klog.V(4).Infof("vSphere CNS driver creating volume %s with create spec %+v", spec.Name, spew.Sdump(createSpec))
	volumeID, err := manager.VolumeManager.CreateVolume(createSpec)
	if err != nil {
		klog.Errorf("Failed to create disk %s with error %+v", spec.Name, err)
		return "", err
	}
	return volumeID.Id, nil
}

// AttachVolumeUtil is the helper function to attach CNS volume to specified vm
func AttachVolumeUtil(ctx context.Context, manager *Manager,
	vm *vsphere.VirtualMachine,
	volumeID string) (string, error) {
	klog.V(4).Infof("vSphere CNS driver is attaching volume: %s to node vm: %s", volumeID, vm.InventoryPath)
	diskUUID, err := manager.VolumeManager.AttachVolume(vm, volumeID)
	if err != nil {
		klog.Errorf("Failed to attach disk %s with err %+v", volumeID, err)
		return "", err
	}
	klog.V(4).Infof("Successfully attached disk %s to VM %v. Disk UUID is %s", volumeID, vm, diskUUID)
	return diskUUID, nil
}

// DetachVolumeUtil is the helper function to detach CNS volume from specified vm
func DetachVolumeUtil(ctx context.Context, manager *Manager,
	vm *vsphere.VirtualMachine,
	volumeID string) error {
	klog.V(4).Infof("vSphere CNS driver is detaching volume: %s from node vm: %s", volumeID, vm.InventoryPath)
	err := manager.VolumeManager.DetachVolume(vm, volumeID)
	if err != nil {
		klog.Errorf("Failed to detach disk %s with err %+v", volumeID, err)
		return err
	}
	klog.V(4).Infof("Successfully detached disk %s from VM %v.", volumeID, vm)
	return nil
}

// DeleteVolumeUtil is the helper function to delete CNS volume for given volumeId
func DeleteVolumeUtil(ctx context.Context, manager *Manager, volumeID string, deleteDisk bool) error {
	var err error
	klog.V(4).Infof("vSphere Cloud Provider deleting volume: %s", volumeID)
	err = manager.VolumeManager.DeleteVolume(volumeID, deleteDisk)
	if err != nil {
		klog.Errorf("Failed to delete disk %s with error %+v", volumeID, err)
		return err
	}
	klog.V(4).Infof("Successfully deleted disk for volumeid: %s", volumeID)
	return nil
}

// Helper function to get DatastoreMoRefs
func getDatastoreMoRefs(datastores []*vsphere.DatastoreInfo) []vim25types.ManagedObjectReference {
	var datastoreMoRefs []vim25types.ManagedObjectReference
	for _, datastore := range datastores {
		datastoreMoRefs = append(datastoreMoRefs, datastore.Reference())
	}
	return datastoreMoRefs
}

