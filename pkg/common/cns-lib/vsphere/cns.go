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

package vsphere

import (
	"context"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/methods"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
)

// Namespace and Path constants
const (
	Namespace = "vsan"
	Path      = "/vsanHealth"
)

var (
	CnsVolumeManagerInstance = vimtypes.ManagedObjectReference{
		Type:  "CnsVolumeManager",
		Value: "cns-volume-manager",
	}
	CnsCnsTaskResultManagerInstance = vimtypes.ManagedObjectReference{
		Type:  "CnsTaskResultManager",
		Value: "cns-task-result-manager",
	}
)

type CNSClient struct {
	*soap.Client
}

// NewCnsClient creates a new CNS client
func NewCnsClient(ctx context.Context, c *vim25.Client) (*CNSClient, error) {
	sc := c.Client.NewServiceClient(Path, Namespace)
	return &CNSClient{sc}, nil
}

// ConnectCns creates a CNS client for the virtual center.
func (vc *VirtualCenter) ConnectCns(ctx context.Context) error {
	var err = vc.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to Virtual Center host %q with err: %v", vc.Config.Host, err)
		return err
	}
	if vc.CnsClient == nil {
		if vc.CnsClient, err = NewCnsClient(ctx, vc.Client.Client); err != nil {
			klog.Errorf("Failed to create CNS client on vCenter host %q with err: %v", vc.Config.Host, err)
			return err
		}
	}
	return nil
}

// DisconnectCns destroys the CNS client for the virtual center.
func (vc *VirtualCenter) DisconnectCns(ctx context.Context) {
	if vc.CnsClient == nil {
		klog.V(1).Info("CnsClient wasn't connected, ignoring")
	} else {
		vc.CnsClient = nil
	}
}

// CreateVolume calls the CNS create API.
func (vc *VirtualCenter) CreateVolume(ctx context.Context, createSpecList []cnstypes.CnsVolumeCreateSpec) (*object.Task, error) {
	req := cnstypes.CnsCreateVolume{
		This:        CnsVolumeManagerInstance,
		CreateSpecs: createSpecList,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsCreateVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(vc.Client.Client, res.Returnval), nil
}

// UpdateVolumeMetadata calls the CNS CnsUpdateVolumeMetadata API with UpdateSpecs specified in the argument
func (vc *VirtualCenter) UpdateVolumeMetadata(ctx context.Context, updateSpecList []cnstypes.CnsVolumeMetadataUpdateSpec) (*object.Task, error) {
	req := cnstypes.CnsUpdateVolumeMetadata{
		This:        CnsVolumeManagerInstance,
		UpdateSpecs: updateSpecList,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsUpdateVolumeMetadata(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(vc.Client.Client, res.Returnval), nil
}

// DeleteVolume calls the CNS delete API.
func (vc *VirtualCenter) DeleteVolume(ctx context.Context, volumeIDList []cnstypes.CnsVolumeId, deleteDisk bool) (*object.Task, error) {
	req := cnstypes.CnsDeleteVolume{
		This:       CnsVolumeManagerInstance,
		VolumeIds:  volumeIDList,
		DeleteDisk: deleteDisk,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsDeleteVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(vc.Client.Client, res.Returnval), nil
}

// AttachVolume calls the CNS Attach API.
func (vc *VirtualCenter) AttachVolume(ctx context.Context, attachSpecList []cnstypes.CnsVolumeAttachDetachSpec) (*object.Task, error) {
	req := cnstypes.CnsAttachVolume{
		This:        CnsVolumeManagerInstance,
		AttachSpecs: attachSpecList,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsAttachVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(vc.Client.Client, res.Returnval), nil
}

// DetachVolume calls the CNS Detach API.
func (vc *VirtualCenter) DetachVolume(ctx context.Context, detachSpecList []cnstypes.CnsVolumeAttachDetachSpec) (*object.Task, error) {
	req := cnstypes.CnsDetachVolume{
		This:        CnsVolumeManagerInstance,
		DetachSpecs: detachSpecList,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsDetachVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return object.NewTask(vc.Client.Client, res.Returnval), nil
}

// QueryVolume calls the CNS QueryVolume API.
func (vc *VirtualCenter) QueryVolume(ctx context.Context, queryFilter cnstypes.CnsQueryFilter) (*cnstypes.CnsQueryResult, error) {
	req := cnstypes.CnsQueryVolume{
		This:   CnsVolumeManagerInstance,
		Filter: queryFilter,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsQueryVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return &res.Returnval, nil
}

// QueryVolume calls the CNS QueryAllVolume API.
func (vc *VirtualCenter) QueryAllVolume(ctx context.Context, queryFilter cnstypes.CnsQueryFilter, querySelection cnstypes.CnsQuerySelection) (*cnstypes.CnsQueryResult, error) {
	req := cnstypes.CnsQueryAllVolume{
		This:      CnsVolumeManagerInstance,
		Filter:    queryFilter,
		Selection: querySelection,
	}
	err := vc.ConnectCns(ctx)
	if err != nil {
		return nil, err
	}
	res, err := methods.CnsQueryAllVolume(ctx, vc.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return &res.Returnval, nil
}
