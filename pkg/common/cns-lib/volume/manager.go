// Copyright 2018 VMware, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package volume

import (
	"context"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"k8s.io/klog"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sync"
)

// Manager provides functionality to manage volumes.
type Manager interface {
	// CreateVolume creates a new volume given its spec.
	CreateVolume(spec *cnstypes.CnsVolumeCreateSpec) (*cnstypes.CnsVolumeId, error)
	// AttachVolume attaches a volume to a virtual machine given the spec.
	AttachVolume(vm *cnsvsphere.VirtualMachine, volumeID string) (string, error)
	// DetachVolume detaches a volume from the virtual machine given the spec.
	DetachVolume(vm *cnsvsphere.VirtualMachine, volumeID string) error
	// DeleteVolume deletes a volume given its spec.
	DeleteVolume(volumeID string, deleteDisk bool) error
	// UpdateVolumeMetadata updates a volume metadata given its spec.
	UpdateVolumeMetadata(spec *cnstypes.CnsVolumeMetadataUpdateSpec) error
	// QueryVolume returns volumes matching the given filter.
	QueryVolume(queryFilter cnstypes.CnsQueryFilter) (*cnstypes.CnsQueryResult, error)
	// QueryAllVolume returns all volumes matching the given filter and selection.
	QueryAllVolume(queryFilter cnstypes.CnsQueryFilter, querySelection cnstypes.CnsQuerySelection) (*cnstypes.CnsQueryResult, error)
}

var (
	// managerInstance is a Manager singleton.
	managerInstance *defaultManager
	// onceForManager is used for initializing the Manager singleton.
	onceForManager sync.Once
)

// GetManager returns the Manager singleton.
func GetManager(vc *cnsvsphere.VirtualCenter) Manager {
	onceForManager.Do(func() {
		klog.V(1).Infof("Initializing volume.defaultManager...")
		managerInstance = &defaultManager{
			virtualCenter: vc,
		}
		klog.V(1).Infof("volume.defaultManager initialized")
	})
	return managerInstance
}

// DefaultManager provides functionality to manage volumes.
type defaultManager struct {
	virtualCenter *cnsvsphere.VirtualCenter
}

// CreateVolume creates a new volume given its spec.
func (m *defaultManager) CreateVolume(spec *cnstypes.CnsVolumeCreateSpec) (*cnstypes.CnsVolumeId, error) {
	err := validateManager(m)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return nil, err
	}

	// If the VSphereUser in the CreateSpec is different from session user, update the CreateSpec
	s, err := m.virtualCenter.Client.SessionManager.UserSession(ctx)
	if err != nil {
		klog.Errorf("Failed to get usersession with err: %v", err)
		return nil, err
	}
	if s.UserName != spec.Metadata.ContainerCluster.VSphereUser {
		klog.V(4).Infof("Update VSphereUser from %s to %s", spec.Metadata.ContainerCluster.VSphereUser, s.UserName)
		spec.Metadata.ContainerCluster.VSphereUser = s.UserName
	}

	// Construct the CNS VolumeCreateSpec list
	var cnsCreateSpecList []cnstypes.CnsVolumeCreateSpec
	cnsCreateSpecList = append(cnsCreateSpecList, *spec)
	// Call the CNS CreateVolume
	task, err := m.virtualCenter.CreateVolume(ctx, cnsCreateSpecList)
	if err != nil {
		klog.Errorf("CNS CreateVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return nil, err
	}
	// Get the taskInfo
	taskInfo, err := GetTaskInfo(ctx, task)
	if err != nil {
		klog.Errorf("Failed to get taskInfo for CreateVolume task from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return nil, err
	}

	// Get the taskResult
	taskResult, err := GetTaskResult(ctx, taskInfo)

	if err != nil {
		klog.Errorf("unable to find the task result for CreateVolume task from vCenter %q with taskID %s, createResults %v",
			m.virtualCenter.Config.Host, taskInfo.Task.Value, taskResult)
		return nil, err
	}

	if taskResult == nil {
		klog.Errorf("taskResult is empty for CreateVolume task")
		return nil, errors.New("taskResult is empty")
	}
	volumeOperationRes := taskResult.GetCnsVolumeOperationResult()
	if volumeOperationRes.Fault != nil {
		klog.Errorf("failed to create cns volume. createSpec: %s, fault: %s", spew.Sdump(spec), spew.Sdump(volumeOperationRes.Fault))
		return nil, errors.New(volumeOperationRes.Fault.LocalizedMessage)
	}
	klog.V(2).Infof("Successfully retrieved the create Result in host %q with taskID %s and volumeID %s",
		m.virtualCenter.Config.Host, taskInfo.Task.Value, volumeOperationRes.VolumeId.Id)

	return &cnstypes.CnsVolumeId{
		Id: volumeOperationRes.VolumeId.Id,
	}, nil
}

// AttachVolume attaches a volume to a virtual machine given the spec.
func (m *defaultManager) AttachVolume(vm *cnsvsphere.VirtualMachine, volumeID string) (string, error) {
	err := validateManager(m)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return "", err
	}
	// Construct the CNS AttachSpec list
	var cnsAttachSpecList []cnstypes.CnsVolumeAttachDetachSpec
	cnsAttachSpec := cnstypes.CnsVolumeAttachDetachSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: volumeID,
		},
		Vm: vm.Reference(),
	}
	cnsAttachSpecList = append(cnsAttachSpecList, cnsAttachSpec)
	// Call the CNS AttachVolume
	task, err := m.virtualCenter.AttachVolume(ctx, cnsAttachSpecList)
	if err != nil {
		klog.Errorf("CNS AttachVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return "", err
	}
	// Get the taskInfo
	taskInfo, err := GetTaskInfo(ctx, task)
	if err != nil {
		klog.Errorf("Failed to get taskInfo for AttachVolume task from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return "", err
	}

	// Get the taskResult
	taskResult, err := GetTaskResult(ctx, taskInfo)
	if err != nil {
		klog.Errorf("unable to find the task result for AttachVolume task from vCenter %q with taskID %s and attachResults %v",
			m.virtualCenter.Config.Host, taskInfo.Task.Value, taskResult)
		return "", err
	}

	if taskResult == nil {
		klog.Errorf("taskResult is empty for AttachVolume task")
		return "", errors.New("taskResult is empty")
	}

	volumeOperationRes := taskResult.GetCnsVolumeOperationResult()
	if volumeOperationRes.Fault != nil {
		if volumeOperationRes.Fault.LocalizedMessage == CNSVolumeResourceInUseFaultMessage {
			// Volume is already attached to VM
			diskUUID, err := IsDiskAttached(ctx, vm, volumeID)
			if err != nil {
				return "", err
			}
			if diskUUID != "" {
				return diskUUID, nil
			}
		}
		klog.Errorf("failed to attach cns volume: %s to node vm: %s. fault: %s", volumeID, vm.InventoryPath, spew.Sdump(volumeOperationRes.Fault))
		return "", errors.New(volumeOperationRes.Fault.LocalizedMessage)
	}
	diskUUID := interface{}(taskResult).(*cnstypes.CnsVolumeAttachResult).DiskUUID
	klog.V(3).Infof("Successfully attached the volume %s to node: %s in vCenter %q with taskID %s and Disk UUID is %s",
		volumeID, vm.InventoryPath, m.virtualCenter.Config.Host, taskInfo.Task.Value, diskUUID)
	return diskUUID, nil
}

// DetachVolume detaches a volume from the virtual machine given the spec.
func (m *defaultManager) DetachVolume(vm *cnsvsphere.VirtualMachine, volumeID string) error {
	err := validateManager(m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return err
	}
	// Construct the CNS DetachSpec list
	var cnsDetachSpecList []cnstypes.CnsVolumeAttachDetachSpec
	cnsDetachSpec := cnstypes.CnsVolumeAttachDetachSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: volumeID,
		},
		Vm: vm.Reference(),
	}
	cnsDetachSpecList = append(cnsDetachSpecList, cnsDetachSpec)
	// Call the CNS DetachVolume
	task, err := m.virtualCenter.DetachVolume(ctx, cnsDetachSpecList)
	if err != nil {
		klog.Errorf("CNS DetachVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the taskInfo
	taskInfo, err := GetTaskInfo(ctx, task)
	if err != nil {
		klog.Errorf("Failed to get taskInfo for DetachVolume task from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the task results for the given task
	taskResult, err := GetTaskResult(ctx, taskInfo)
	if err != nil {
		klog.Errorf("unable to find the task result for DetachVolume task from vCenter %q with taskID %s and detachResults %v",
			m.virtualCenter.Config.Host, taskInfo.Task.Value, taskResult)
		return err
	}

	if taskResult == nil {
		klog.Errorf("taskResult is empty for DetachVolume task")
		return errors.New("taskResult is empty")
	}

	volumeOperationRes := taskResult.GetCnsVolumeOperationResult()

	if volumeOperationRes.Fault != nil {
		klog.Errorf("failed to detach cns volume:%s from node vm: %s. fault: %s", volumeID, vm.InventoryPath, spew.Sdump(volumeOperationRes.Fault))
		return errors.New(volumeOperationRes.Fault.LocalizedMessage)
	}

	klog.V(3).Infof("Successfully detached the volume %s from node: %s in vCenter %q with taskID %s",
		volumeID, vm.InventoryPath, m.virtualCenter.Config.Host, taskInfo.Task.Value)
	return nil
}

// DeleteVolume deletes a volume given its spec.
func (m *defaultManager) DeleteVolume(volumeID string, deleteDisk bool) error {
	err := validateManager(m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return err
	}
	// Construct the CNS VolumeId list
	var cnsVolumeIDList []cnstypes.CnsVolumeId
	cnsVolumeID := cnstypes.CnsVolumeId{
		Id: volumeID,
	}
	cnsVolumeIDList = append(cnsVolumeIDList, cnsVolumeID)
	// Call the CNS DeleteVolume
	task, err := m.virtualCenter.DeleteVolume(ctx, cnsVolumeIDList, deleteDisk)
	if err != nil {
		klog.Errorf("CNS DeleteVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the taskInfo
	taskInfo, err := GetTaskInfo(ctx, task)
	if err != nil {
		klog.Errorf("Failed to get taskInfo for DeleteVolume task from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the task results for the given task
	taskResult, err := GetTaskResult(ctx, taskInfo)
	if err != nil {
		klog.Errorf("unable to find the task result for DeleteVolume task from vCenter %q with taskID %s and deleteResults %v",
			m.virtualCenter.Config.Host, taskInfo.Task.Value, taskResult)
		return err
	}
	if taskResult == nil {
		klog.Errorf("taskResult is empty for DeleteVolume task")
		return errors.New("taskResult is empty")
	}

	volumeOperationRes := taskResult.GetCnsVolumeOperationResult()
	if volumeOperationRes.Fault != nil {
		klog.Errorf("Failed to delete volume: %s, fault: %s", volumeID, spew.Sdump(volumeOperationRes.Fault))
		return errors.New(volumeOperationRes.Fault.LocalizedMessage)
	}

	klog.V(3).Infof("Successfully deleted the volume %s from vCenter %q with taskID %s", volumeID,
		m.virtualCenter.Config.Host, taskInfo.Task.Value)
	return nil
}

// UpdateVolume updates a volume given its spec.
func (m *defaultManager) UpdateVolumeMetadata(spec *cnstypes.CnsVolumeMetadataUpdateSpec) error {
	err := validateManager(m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return err
	}
	// If the VSphereUser in the VolumeMetadataUpdateSpec is different from session user, update the VolumeMetadataUpdateSpec
	s, err := m.virtualCenter.Client.SessionManager.UserSession(ctx)
	if err != nil {
		klog.Errorf("Failed to get usersession with err: %v", err)
		return err
	}
	if s.UserName != spec.Metadata.ContainerCluster.VSphereUser {
		klog.V(4).Infof("Update VSphereUser from %s to %s", spec.Metadata.ContainerCluster.VSphereUser, s.UserName)
		spec.Metadata.ContainerCluster.VSphereUser = s.UserName
	}

	var cnsUpdateSpecList []cnstypes.CnsVolumeMetadataUpdateSpec
	cnsUpdateSpec := cnstypes.CnsVolumeMetadataUpdateSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: spec.VolumeId.Id,
		},
		Metadata: spec.Metadata,
	}
	cnsUpdateSpecList = append(cnsUpdateSpecList, cnsUpdateSpec)
	task, err := m.virtualCenter.UpdateVolumeMetadata(ctx, cnsUpdateSpecList)
	if err != nil {
		klog.Errorf("CNS UpdateVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the taskInfo
	taskInfo, err := GetTaskInfo(ctx, task)
	if err != nil {
		klog.Errorf("Failed to get taskInfo for UpdateVolume task from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return err
	}
	// Get the task results for the given task
	taskResult, err := GetTaskResult(ctx, taskInfo)
	if err != nil {
		klog.Errorf("unable to find the task result for UpdateVolume task from vCenter %q with taskID %s and updateResults %v",
			m.virtualCenter.Config.Host, taskInfo.Task.Value, taskResult)
		return err
	}

	if taskResult == nil {
		klog.Errorf("taskResult is empty for UpdateVolume task")
		return errors.New("taskResult is empty")
	}
	volumeOperationRes := taskResult.GetCnsVolumeOperationResult()
	if volumeOperationRes.Fault != nil {
		klog.Errorf("Failed to update volume. updateSpec: %s, fault: %s", spew.Sdump(spec), spew.Sdump(volumeOperationRes.Fault))
		return errors.New(volumeOperationRes.Fault.LocalizedMessage)
	}
	klog.V(3).Infof("Successfully retrieved the update metadata for volume %s with spec %s, host %q and taskID %s",
		spec.VolumeId.Id, spew.Sdump(spec), m.virtualCenter.Config.Host, taskInfo.Task.Value)

	return nil
}

// QueryVolume returns volumes matching the given filter.
func (m *defaultManager) QueryVolume(queryFilter cnstypes.CnsQueryFilter) (*cnstypes.CnsQueryResult, error) {
	err := validateManager(m)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return nil, err
	}
	//Call the CNS QueryVolume
	res, err := m.virtualCenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		klog.Errorf("CNS QueryVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return nil, err
	}
	return res, err
}

// QueryAllVolume returns all volumes matching the given filter and selection.
func (m *defaultManager) QueryAllVolume(queryFilter cnstypes.CnsQueryFilter, querySelection cnstypes.CnsQuerySelection) (*cnstypes.CnsQueryResult, error) {
	err := validateManager(m)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Set up the VC connection
	err = m.virtualCenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center with err: %v", err)
		return nil, err
	}
	//Call the CNS QueryAllVolume
	res, err := m.virtualCenter.QueryAllVolume(ctx, queryFilter, querySelection)
	if err != nil {
		klog.Errorf("CNS QueryAllVolume failed from vCenter %q with err: %v", m.virtualCenter.Config.Host, err)
		return nil, err
	}
	return res, err
}
