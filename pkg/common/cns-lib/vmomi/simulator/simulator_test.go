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
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"
	vim25types "github.com/vmware/govmomi/vim25/types"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	cnsvolume "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
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

	// Query
	queryFilter = cnstypes.CnsQueryFilter{}
	queryResult, err = cnsClient.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != existingNumDisks+1 {
		t.Fatal("Number of volumes mismatches after creating a single volume")
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
