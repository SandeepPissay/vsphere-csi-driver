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

package vanilla

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/vmware/govmomi/units"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	cspvolume "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	csitypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	"strings"
)

var (
	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
	}
)

type NodeManagerInterface interface {
	Initialize() error
	GetSharedDatastoresInK8SCluster(ctx context.Context) ([]*cnsvsphere.DatastoreInfo, error)
	GetSharedDatastoresInTopology(ctx context.Context, topologyRequirement *csi.TopologyRequirement, zoneKey string, regionKey string) ([]*cnsvsphere.DatastoreInfo, map[string]string, error)
	GetNodeByName(nodeName string) (*cnsvsphere.VirtualMachine, error)
}

type controller struct {
	manager *block.Manager
	nodeMgr NodeManagerInterface
}

// New creates a CNS controller
func New() csitypes.CnsController {
	return &controller{}
}

// Init is initializing controller struct
func (c *controller) Init(config *config.Config) error {
	klog.Infof("Initializing CNS controller")
	// Get VirtualCenterManager instance and validate version
	var err error
	vcenterconfig, err := cnsvsphere.GetVirtualCenterConfig(config)
	if err != nil {
		klog.Errorf("Failed to get VirtualCenterConfig. err=%v", err)
		return err
	}
	vcManager := cnsvsphere.GetVirtualCenterManager()
	vcenter, err := vcManager.RegisterVirtualCenter(vcenterconfig)
	if err != nil {
		klog.Errorf("Failed to register VC with virtualCenterManager. err=%v", err)
		return err
	}
	c.manager = &block.Manager{
		VcenterConfig:  vcenterconfig,
		CnsConfig:      config,
		VolumeManager:  cspvolume.GetManager(vcenter),
		VcenterManager: cnsvsphere.GetVirtualCenterManager(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vc, err := block.GetVCenter(ctx, c.manager)
	if err != nil {
		klog.Errorf("Failed to get vcenter. err=%v", err)
		return err
	}
	// Check vCenter API Version
	if err = block.CheckAPI(vc.Client.ServiceContent.About.ApiVersion); err != nil {
		klog.Errorf("checkAPI failed for vcenter API version: %s, err=%v", vc.Client.ServiceContent.About.ApiVersion, err)
		return err
	}
	c.nodeMgr = &Nodes{}
	err = c.nodeMgr.Initialize()
	if err != nil {
		klog.Errorf("Failed to initialize nodeMgr. err=%v", err)
	}
	return nil
}

// CreateVolume is creating CNS Volume using volume request specified
// in CreateVolumeRequest
func (c *controller) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (
	*csi.CreateVolumeResponse, error) {

	klog.V(4).Infof("CreateVolume: called with args %+v", *req)
	err := validateVanillaCreateVolumeRequest(req)
	if err != nil {
		klog.Errorf("Failed to validate Create Volume Request with err: %v", err)
		return nil, err
	}

	volSizeBytes := int64(req.GetCapacityRange().GetRequiredBytes())
	volSizeMB := int64(block.RoundUpSize(volSizeBytes, block.MbInBytes))

	var datastoreURL string
	var storagePolicyName string
	var fsType string

	// Support case insensitive parameters
	for paramName := range req.Parameters {
		param := strings.ToLower(paramName)
		if param == block.AttributeDatastoreURL {
			datastoreURL = req.Parameters[paramName]
		} else if param == block.AttributeStoragePolicyName {
			storagePolicyName = req.Parameters[paramName]
		} else if param == block.AttributeFsType {
			fsType = req.Parameters[block.AttributeFsType]
		}
	}

	var createVolumeSpec = block.CreateVolumeSpec{
		CapacityMB:        volSizeMB,
		Name:              req.Name,
		DatastoreURL:      datastoreURL,
		StoragePolicyName: storagePolicyName,
	}
	var sharedDatastores []*cnsvsphere.DatastoreInfo

	// Get accessibility
	topologyRequirement := req.GetAccessibilityRequirements()
	var volumeAccessibleTopology = make(map[string]string)
	if topologyRequirement != nil {
		// Get shared accessible datastores for matching topology requirement
		if c.manager.CnsConfig.Labels.Zone == "" || c.manager.CnsConfig.Labels.Region == "" {
			// if zone and region label (vsphere categoy names) not specified in the csi-config configmap, then return
			// NotFound error.
			errMsg := fmt.Sprintf("Zone/Region vsphere category names not specified in the csi credentials secret")
			klog.Errorf(errMsg)
			return nil, status.Error(codes.NotFound, errMsg)
		}
		sharedDatastores, volumeAccessibleTopology, err = c.nodeMgr.GetSharedDatastoresInTopology(ctx, topologyRequirement, c.manager.CnsConfig.Labels.Zone, c.manager.CnsConfig.Labels.Region)
		if err != nil {
			msg := fmt.Sprintf("Failed to get shared datastores in topology. Error: %+v", err)
			klog.Errorf(msg)
			return nil, err
		}
		if createVolumeSpec.DatastoreURL != "" && len(volumeAccessibleTopology) != 0 {
			// Check datastoreURL specified in the storageclass is accessible from topology
			isDataStoreAccessible := false
			for _, sharedDatastore := range sharedDatastores {
				if sharedDatastore.Info.Url == createVolumeSpec.DatastoreURL {
					isDataStoreAccessible = true
					break
				}
			}
			if !isDataStoreAccessible {
				errMsg := fmt.Sprintf("DatastoreURL: %s specified in the storage class is not accessible from  zone [%s] "+
					"and region [%s]", createVolumeSpec.DatastoreURL, volumeAccessibleTopology[csitypes.LabelZoneFailureDomain], volumeAccessibleTopology[csitypes.LabelRegionFailureDomain])
				klog.Errorf(errMsg)
				return nil, status.Error(codes.InvalidArgument, errMsg)
			}
		}

	} else {
		// Get shared datastores for the Kubernetes cluster
		sharedDatastores, err = c.nodeMgr.GetSharedDatastoresInK8SCluster(ctx)
		if err != nil {
			msg := fmt.Sprintf("Failed to get shared datastores in kubernetes cluster. Error: %+v", err)
			klog.Errorf(msg)
			return nil, err
		}
	}
	volumeID, err := block.CreateVolumeUtil(ctx, c.manager, &createVolumeSpec, sharedDatastores)
	if err != nil {
		msg := fmt.Sprintf("Failed to create volume. Error: %+v", err)
		klog.Errorf(msg)
		return nil, err
	}
	attributes := make(map[string]string)
	attributes[block.AttributeDiskType] = block.DiskTypeString
	attributes[block.AttributeFsType] = fsType
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: int64(units.FileSize(volSizeMB * block.MbInBytes)),
			VolumeContext: attributes,
		},
	}
	if len(volumeAccessibleTopology) != 0 {
		volumeTopology := &csi.Topology{
			Segments: volumeAccessibleTopology,
		}
		resp.Volume.AccessibleTopology = append(resp.Volume.AccessibleTopology, volumeTopology)
	}
	return resp, nil
}

// CreateVolume is deleting CNS Volume specified in DeleteVolumeRequest
func (c *controller) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (
	*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume: called with args: %+v", *req)
	var err error
	err = validateVanillaDeleteVolumeRequest(req)
	if err != nil {
		return nil, err
	}
	err = block.DeleteVolumeUtil(ctx, c.manager, req.VolumeId, true)
	if err != nil {
		msg := fmt.Sprintf("Failed to delete volume: %q. Error: %+v", req.VolumeId, err)
		klog.Error(msg)
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches a volume to the Node VM.
// volume id and node name is retrieved from ControllerPublishVolumeRequest
func (c *controller) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (
	*csi.ControllerPublishVolumeResponse, error) {

	klog.V(4).Infof("ControllerPublishVolume: called with args %+v", *req)
	err := validateControllerPublishVolumeRequest(req)
	if err != nil {
		return nil, err
	}
	node, err := c.nodeMgr.GetNodeByName(req.NodeId)
	if err != nil {
		msg := fmt.Sprintf("Failed to find VirtualMachine for node:%q. Error: %v", req.NodeId, err)
		klog.Error(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}
	diskUUID, err := block.AttachVolumeUtil(ctx, c.manager, node, req.VolumeId)
	if err != nil {
		msg := fmt.Sprintf("Failed to attach disk: %+q with node: %q err %+v", req.VolumeId, req.NodeId, err)
		klog.Error(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}
	publishInfo := make(map[string]string, 0)
	publishInfo[block.AttributeDiskType] = block.DiskTypeString
	publishInfo[block.AttributeFirstClassDiskPage83Data] = block.FormatDiskUUID(diskUUID)
	resp := &csi.ControllerPublishVolumeResponse{
		PublishContext: publishInfo,
	}
	return resp, nil
}

// ControllerUnpublishVolume detaches a volume from the Node VM.
// volume id and node name is retrieved from ControllerUnpublishVolumeRequest
func (c *controller) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (
	*csi.ControllerUnpublishVolumeResponse, error) {

	klog.V(4).Infof("ControllerUnpublishVolume: called with args %+v", *req)
	err := validateControllerUnpublishVolumeRequest(req)
	if err != nil {
		msg := fmt.Sprintf("Failed to connect to virtual center. Error: %v", err)
		klog.Error(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}
	node, err := c.nodeMgr.GetNodeByName(req.NodeId)
	if err != nil {
		msg := fmt.Sprintf("Failed to find VirtualMachine for node:%q. Error: %v", req.NodeId, err)
		klog.Error(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}
	err = block.DetachVolumeUtil(ctx, c.manager, node, req.VolumeId)
	if err != nil {
		msg := fmt.Sprintf("Failed to detach disk: %+q from node: %q err %+v", req.VolumeId, req.NodeId, err)
		klog.Error(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}
	resp := &csi.ControllerUnpublishVolumeResponse{}
	return resp, nil
}

// ValidateVolumeCapabilities returns the capabilities of the volume.
func (c *controller) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (
	*csi.ValidateVolumeCapabilitiesResponse, error) {

	klog.V(4).Infof("ControllerGetCapabilities: called with args %+v", *req)
	volCaps := req.GetVolumeCapabilities()
	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if block.IsValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

func (c *controller) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (
	*csi.ListVolumesResponse, error) {

	klog.V(4).Infof("ListVolumes: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *controller) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (
	*csi.GetCapacityResponse, error) {

	klog.V(4).Infof("GetCapacity: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *controller) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (
	*csi.ControllerGetCapabilitiesResponse, error) {

	klog.V(4).Infof("ControllerGetCapabilities: called with args %+v", *req)
	var caps []*csi.ControllerServiceCapability
	for _, cap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (c *controller) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (
	*csi.CreateSnapshotResponse, error) {

	klog.V(4).Infof("CreateSnapshot: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *controller) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (
	*csi.DeleteSnapshotResponse, error) {

	klog.V(4).Infof("DeleteSnapshot: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (c *controller) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (
	*csi.ListSnapshotsResponse, error) {

	klog.V(4).Infof("ListSnapshots: called with args %+v", *req)
	return nil, status.Error(codes.Unimplemented, "")
}
