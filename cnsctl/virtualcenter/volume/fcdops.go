/*
Copyright 2020 The Kubernetes Authors.

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
package volume

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vslm"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/vm"
)

type DeleteFcdRequest struct {
	Client     *govmomi.Client
	FcdId      string
	Datastore  string
	Datacenter string
}

// Deletes the FCD.
// If forceDelete is true and if the FCD is attached to a VM, it detaches it and then deletes it.
// If forceDelete is false, the FCD is not deleted.
func DeleteFcd(ctx context.Context, req *DeleteFcdRequest, forceDelete string) (bool, error) {
	finder := find.NewFinder(req.Client.Client, false)
	dcObj, err := finder.Datacenter(ctx, req.Datacenter)
	if err != nil {
		fmt.Printf("Unable to find datacenter: %s\n", req.Datacenter)
		return false, err
	}
	finder.SetDatacenter(dcObj)
	dsObj, err := finder.Datastore(ctx, req.Datastore)
	if err != nil {
		fmt.Printf("Unable to find datastore: %s\n", req.Datastore)
		return false, err
	}
	m := vslm.NewObjectManager(req.Client.Client)
	retObjAsso := &types.RetrieveVStorageObjectAssociations{
		This: m.Reference(),
		Ids: []types.RetrieveVStorageObjSpec{
			{
				Id:        types.ID{Id: req.FcdId},
				Datastore: dsObj.Reference(),
			},
		},
	}
	res, err := methods.RetrieveVStorageObjectAssociations(ctx, req.Client.RoundTripper, retObjAsso)
	if err != nil {
		fmt.Printf("Failed to get VM associations for FCD: %s\n", req.FcdId)
		return false, err
	}
	if len(res.Returnval) > 0 && len(res.Returnval[0].VmDiskAssociations) > 0 {
		vmId := res.Returnval[0].VmDiskAssociations[0].VmId
		vmMo, err := vm.GetVirtualMachine(ctx, req.Client.Client, vmId)
		if err != nil {
			fmt.Printf("FCD %s is attached to VM. Failed to get the VM. Err: %+v\n", req.FcdId, err)
			return false, err
		}
		if forceDelete == "false" {
			fmt.Printf("FCD %s is attached to VM: %s. Ignoring delete operation.\n", req.FcdId, vmMo.Name)
			return false, nil
		}
		fmt.Printf("FCD %s is attached to VM: %s. Detaching the FCD before deleting it.\n", req.FcdId, vmMo.Name)
		vm := object.NewVirtualMachine(req.Client.Client, types.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: vmId,
		})
		err = vm.DetachDisk(ctx, req.FcdId)
		if err != nil {
			fmt.Printf("Failed to detach FCD %s from VM %s\n", req.FcdId, vmId)
			return false, err
		}
	}

	task, err := m.Delete(ctx, dsObj, req.FcdId)
	if err != nil {
		fmt.Printf("Unable to delete the FCD: %s\n", req.FcdId)
		return false, err
	}
	_, err = task.WaitForResult(ctx)
	if err != nil {
		fmt.Printf("Error while waiting for task result: %+v\n", err)
		return false, err
	}
	fmt.Printf("Deleted FCD %s successfully\n", req.FcdId)
	return true, nil
}
