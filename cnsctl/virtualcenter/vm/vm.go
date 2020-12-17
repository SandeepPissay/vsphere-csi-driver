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

package vm

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Returns the VirtualMachine managed object for the given vmId.
func GetVirtualMachine(ctx context.Context, vcClient *vim25.Client, vmId string) (*mo.VirtualMachine, error) {
	vmObj := object.NewVirtualMachine(vcClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: vmId})
	var vmMo mo.VirtualMachine
	err := vmObj.Properties(ctx, vmObj.Reference(), []string{"name"}, &vmMo)
	if err != nil {
		fmt.Printf("Failed to get the VM managed object for vmId: %s\n", vmId)
		return nil, err
	}
	return &vmMo, nil
}
