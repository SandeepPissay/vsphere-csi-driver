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
	cnstypes "gitlab.eng.vmware.com/hatchway/common-csp/cns/types"
	cnsvolumetypes "gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume/types"
	"gitlab.eng.vmware.com/hatchway/common-csp/pkg/vsphere"
	"golang.org/x/net/context"
	"k8s.io/klog"
)

// Helper function to create CNS volume
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
			var datastoreUrl string
			datastoreUrl, err = datastoreObj.GetDatastoreUrl(ctx)
			if err != nil {
				klog.Errorf("Failed to get URL for the datastore:%s , Error: %+v", spec.Datastore, err)
				return "", err
			}
			for _, sharedDatastore := range sharedDatastores {
				if sharedDatastore.Info.Url == datastoreUrl {
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
	createSpec := &cnsvolumetypes.CreateSpec{
		Name:       spec.Name,
		Datastores: datastores,
		BackingInfo: &cnsvolumetypes.BlockBackingInfo{
			BackingObjectInfo: cnsvolumetypes.BackingObjectInfo{
				Capacity: spec.CapacityMB,
			},
		},
		ContainerCluster: getContainerCluster(manager.CnsConfig.Global.ClusterID, manager.CnsConfig.VirtualCenter[vc.Config.Host].User),
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
	return volumeID.ID, nil
}

// Helper function to attach CNS volume to specified vm
func AttachVolumeUtil(ctx context.Context, manager *Manager,
	vm *vsphere.VirtualMachine,
	volumeId string) (string, error) {

	attachSpec := &cnsvolumetypes.AttachDetachSpec{
		VolumeID: &cnsvolumetypes.VolumeID{
			ID: volumeId,
		},
		VirtualMachine: vm,
	}
	klog.V(4).Infof("vSphere CNS driver is attaching volume %s with attach spec %s", volumeId, spew.Sdump(attachSpec))
	diskUUID, err := manager.VolumeManager.AttachVolume(attachSpec)
	if err != nil {
		klog.Errorf("Failed to attach disk %s with err %+v", volumeId, err)
		return "", err
	}
	klog.V(4).Infof("Successfully attached disk %s to VM %v. Disk UUID is %s", volumeId, vm, diskUUID)
	return diskUUID, nil
}

// Helper function to detach CNS volume from specified vm
func DetachVolumeUtil(ctx context.Context, manager *Manager,
	vm *vsphere.VirtualMachine,
	volumeId string) error {

	detachSpec := &cnsvolumetypes.AttachDetachSpec{
		VolumeID: &cnsvolumetypes.VolumeID{
			ID: volumeId,
		},
		VirtualMachine: vm,
	}
	klog.V(4).Infof("vSphere CNS driver is detaching volume %s with detachSpec spec %s", volumeId, spew.Sdump(detachSpec))
	err := manager.VolumeManager.DetachVolume(detachSpec)
	if err != nil {
		klog.Errorf("Failed to detach disk %s with err %+v", volumeId, err)
		return err
	}
	klog.V(4).Infof("Successfully detached disk %s from VM %v.", volumeId, vm)
	return nil
}

// Helper function to delete CNS volume for given volumeId
func DeleteVolumeUtil(ctx context.Context, manager *Manager, volumeId string, deleteDisk bool) error {
	var err error
	deleteSpec := &cnsvolumetypes.DeleteSpec{
		VolumeID: &cnsvolumetypes.VolumeID{
			ID: volumeId,
		},
		DeleteDisk: deleteDisk,
	}
	klog.V(4).Infof("vSphere Cloud Provider deleting volume %s with delete spec %s", volumeId, spew.Sdump(deleteSpec))
	err = manager.VolumeManager.DeleteVolume(deleteSpec)
	if err != nil {
		klog.Errorf("Failed to delete disk %s with error %+v", volumeId, err)
		return err
	}
	klog.V(4).Infof("Successfully deleted disk for volumeid: %s", volumeId)
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

// Helper function to create ContainerCluster object
func getContainerCluster(clusterid string, username string) cnsvolumetypes.ContainerCluster {
	return cnsvolumetypes.ContainerCluster{
		ClusterID:   clusterid,
		ClusterType: cnstypes.CnsClusterTypeKubernetes,
		VSphereUser: username,
	}
}
