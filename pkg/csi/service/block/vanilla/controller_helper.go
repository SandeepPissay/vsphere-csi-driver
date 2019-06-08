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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	"strings"
)

const (
	// For more information please see
	// https://kubernetes.io/docs/reference/kubernetes-api/labels-annotations-taints/#failure-domain-beta-kubernetes-io-region.
	// LabelZoneRegion is label placed on nodes and PV containing region detail
	LabelZoneRegion = "failure-domain.beta.kubernetes.io/region"
	// LabelZoneFailureDomain is label placed on nodes and PV containing zone detail
	LabelZoneFailureDomain = "failure-domain.beta.kubernetes.io/zone"
)

// validateVanillaCreateVolumeRequest is the helper function to validate
// CreateVolumeRequest for Vanilla CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateVanillaCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	// Get create params
	params := req.GetParameters()
	for paramName := range params {
		paramName = strings.ToLower(paramName)
		if paramName != block.AttributeDatastoreURL && paramName != block.AttributeStoragePolicyName {
			msg := fmt.Sprintf("Volume parameter %s is not a valid Vanilla CSI parameter.", paramName)
			return status.Error(codes.InvalidArgument, msg)
		}
	}
	return block.ValidateCreateVolumeRequest(req)
}

// validateVanillaDeleteVolumeRequest is the helper function to validate
// DeleteVolumeRequest for Vanilla CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateVanillaDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	return block.ValidateDeleteVolumeRequest(req)

}

// validateControllerPublishVolumeRequest is the helper function to validate
// ControllerPublishVolumeRequest. Function returns error if validation fails otherwise returns nil.
func validateControllerPublishVolumeRequest(req *csi.ControllerPublishVolumeRequest) error {
	//check for required parameters
	if len(req.VolumeId) == 0 {
		msg := "Volume ID is a required parameter."
		klog.Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	} else if len(req.NodeId) == 0 {
		msg := "Node ID is a required parameter."
		klog.Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return status.Error(codes.InvalidArgument, "Volume capability not provided")
	}
	caps := []*csi.VolumeCapability{volCap}
	if !block.IsValidVolumeCapabilities(caps) {
		return status.Error(codes.InvalidArgument, "Volume capability not supported")
	}
	return nil
}

// validateControllerUnpublishVolumeRequest is the helper function to validate
// ControllerUnpublishVolumeRequest. Function returns error if validation fails otherwise returns nil.
func validateControllerUnpublishVolumeRequest(req *csi.ControllerUnpublishVolumeRequest) error {
	//check for required parameters
	if len(req.VolumeId) == 0 {
		msg := "Volume ID is a required parameter."
		klog.Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	} else if len(req.NodeId) == 0 {
		msg := "Node ID is a required parameter."
		klog.Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	return nil
}
