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
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
	vim25types "github.com/vmware/govmomi/vim25/types"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	cnsvolume "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"testing"
)

const (
	testLabel = "testLabel"
	testValue = "testValue"
)

func TestSimulator(t *testing.T) {
	ctx := context.Background()

	model := simulator.VPX()
	defer model.Remove()

	var err error

	if err = model.Create(); err != nil {
		t.Fatal(err)
	}

	s := model.Service.NewServer()
	defer s.Close()

	model.Service.RegisterSDK(New())

	c, err := govmomi.NewClient(ctx, s.URL, true)
	if err != nil {
		t.Fatal(err)
	}

	cnsClient := cnsvsphere.VirtualCenter{
		Client: c,
	}

	err = cnsClient.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Query
	queryFilter := cnstypes.CnsQueryFilter{}
	queryResult, err := cnsClient.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}
	existingNumDisks := len(queryResult.Volumes)

	// Get a simulator DS
	datastore := simulator.Map.Any("Datastore").(*simulator.Datastore)

	// Create
	createSpecList := []cnstypes.CnsVolumeCreateSpec{
		{
			Name:       "test",
			VolumeType: "TestVolumeType",
			Datastores: []vim25types.ManagedObjectReference{
				datastore.Self,
			},
			BackingObjectDetails: &cnstypes.CnsBackingObjectDetails{
				CapacityInMb: 1024,
			},
			Profile: []vim25types.BaseVirtualMachineProfileSpec{
				&vim25types.VirtualMachineDefinedProfileSpec{
					ProfileId: uuid.New().String(),
				},
			},
		},
	}
	createTask, err := cnsClient.CreateVolume(ctx, createSpecList)
	if err != nil {
		t.Fatal(err)
	}

	createTaskInfo, err := cnsvolume.GetTaskInfo(ctx, createTask)
	if err != nil {
		t.Fatal(err)
	}

	createTaskResult, err := cnsvolume.GetTaskResult(ctx, createTaskInfo)
	if err != nil {
		t.Fatal(err)
	}
	if createTaskResult == nil {
		t.Fatalf("Empty create task results")
	}

	createVolumeOperationRes := createTaskResult.GetCnsVolumeOperationResult()
	if createVolumeOperationRes.Fault != nil {
		t.Fatalf("Failed to create volume: fault=%s", spew.Sdump(createVolumeOperationRes.Fault))
	}
	volumeId := createVolumeOperationRes.VolumeId.Id

	// Attach
	// Get a simulator DS
	nodeVM := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)

	attachSpecList := []cnstypes.CnsVolumeAttachDetachSpec{
		{
			VolumeId: createVolumeOperationRes.VolumeId,
			Vm:       nodeVM.Self,
		},
	}
	attachTask, err := cnsClient.AttachVolume(ctx, attachSpecList)
	if err != nil {
		t.Fatal(err)
	}

	attachTaskInfo, err := cnsvolume.GetTaskInfo(ctx, attachTask)
	if err != nil {
		t.Fatal(err)
	}

	attachTaskResult, err := cnsvolume.GetTaskResult(ctx, attachTaskInfo)
	if err != nil {
		t.Fatal(err)
	}
	if attachTaskResult == nil {
		t.Fatalf("Empty attach task results")
	}

	attachVolumeOperationRes := attachTaskResult.GetCnsVolumeOperationResult()
	if attachVolumeOperationRes.Fault != nil {
		t.Fatalf("Failed to attach: fault=%s", spew.Sdump(attachVolumeOperationRes.Fault))
	}

	// Detach
	// Delete
	detachVolumeList := []cnstypes.CnsVolumeAttachDetachSpec{
		{
			VolumeId: createVolumeOperationRes.VolumeId,
		},
	}
	detachTask, err := cnsClient.DetachVolume(ctx, detachVolumeList)

	detachTaskInfo, err := cnsvolume.GetTaskInfo(ctx, detachTask)
	if err != nil {
		t.Fatal(err)
	}

	detachTaskResult, err := cnsvolume.GetTaskResult(ctx, detachTaskInfo)
	if err != nil {
		t.Fatal(err)
	}
	if detachTaskResult == nil {
		t.Fatalf("Empty detach task results")
	}

	detachVolumeOperationRes := detachTaskResult.GetCnsVolumeOperationResult()
	if detachVolumeOperationRes.Fault != nil {
		t.Fatalf("Failed to detach volume: fault=%s", spew.Sdump(detachVolumeOperationRes.Fault))
	}

	// Query
	queryFilter = cnstypes.CnsQueryFilter{}
	queryResult, err = cnsClient.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != existingNumDisks+1 {
		t.Fatal("Number of volumes mismatches after creating a single volume")
	}

	// QueryAll
	queryFilter = cnstypes.CnsQueryFilter{}
	querySelection := cnstypes.CnsQuerySelection{}
	queryResult, err = cnsClient.QueryAllVolume(ctx, queryFilter, querySelection)

	if len(queryResult.Volumes) != existingNumDisks+1 {
		t.Fatal("Number of volumes mismatches after creating a single volume")
	}

	// Update
	var metadataList []cnstypes.BaseCnsEntityMetadata
	newLabels := []types.KeyValue{
		{
			Key:   testLabel,
			Value: testValue,
		},
	}
	metadata := &cnstypes.CnsKubernetesEntityMetadata{

		CnsEntityMetadata: cnstypes.CnsEntityMetadata{
			DynamicData: vim25types.DynamicData{},
			EntityName:  queryResult.Volumes[0].Name,
			Labels:      newLabels,
			Delete:      false,
		},
		EntityType: string(cnstypes.CnsKubernetesEntityTypePV),
		Namespace:  "",
	}
	metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(metadata))
	updateSpecList := []cnstypes.CnsVolumeMetadataUpdateSpec{
		{
			DynamicData: vim25types.DynamicData{},
			VolumeId:    createVolumeOperationRes.VolumeId,
			Metadata: cnstypes.CnsVolumeMetadata{
				DynamicData:      vim25types.DynamicData{},
				ContainerCluster: queryResult.Volumes[0].Metadata.ContainerCluster,
				EntityMetadata:   metadataList,
			},
		},
	}
	updateTask, err := cnsClient.UpdateVolumeMetadata(ctx, updateSpecList)
	if err != nil {
		t.Fatal(err)
	}
	updateTaskInfo, err := cnsvolume.GetTaskInfo(ctx, updateTask)
	if err != nil {
		t.Fatal(err)
	}
	updateTaskResult, err := cnsvolume.GetTaskResult(ctx, updateTaskInfo)
	if err != nil {
		t.Fatal(err)
	}
	if updateTaskResult == nil {
		t.Fatalf("Empty create task results")
	}

	updateVolumeOperationRes := updateTaskResult.GetCnsVolumeOperationResult()
	if updateVolumeOperationRes.Fault != nil {
		t.Fatalf("Failed to create volume: fault=%s", spew.Sdump(updateVolumeOperationRes.Fault))
	}

	// Delete
	deleteVolumeList := []cnstypes.CnsVolumeId{
		{
			Id: volumeId,
		},
	}
	deleteTask, err := cnsClient.DeleteVolume(ctx, deleteVolumeList, true)

	deleteTaskInfo, err := cnsvolume.GetTaskInfo(ctx, deleteTask)
	if err != nil {
		t.Fatal(err)
	}

	deleteTaskResult, err := cnsvolume.GetTaskResult(ctx, deleteTaskInfo)
	if err != nil {
		t.Fatal(err)
	}
	if deleteTaskResult == nil {
		t.Fatalf("Empty delete task results")
	}

	deleteVolumeOperationRes := deleteTaskResult.GetCnsVolumeOperationResult()
	if deleteVolumeOperationRes.Fault != nil {
		t.Fatalf("Failed to delete volume: fault=%s", spew.Sdump(deleteVolumeOperationRes.Fault))
	}

	queryFilter = cnstypes.CnsQueryFilter{}
	queryResult, err = cnsClient.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatalf("Failed to query volume with QueryFilter: err=%v", err)
	}
	if len(queryResult.Volumes) != existingNumDisks {
		t.Fatal("Number of volumes mismatches after deleting a single volume")
	}

}
