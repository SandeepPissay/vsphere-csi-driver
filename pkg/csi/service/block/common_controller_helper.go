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
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
	"strconv"
	"strings"
)

// ValidateCreateVolumeRequest is the helper function to validate
// CreateVolumeRequest for all block controllers.
// Function returns error if validation fails otherwise returns nil.
func ValidateCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	// Volume Name
	volName := req.GetName()
	if len(volName) == 0 {
		msg := "Volume name is a required parameter."
		klog.Error(msg)
		return status.Error(codes.InvalidArgument, msg)
	}
	// Validate Volume Capabilities
	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}
	if !IsValidVolumeCapabilities(volCaps) {
		return status.Error(codes.InvalidArgument, "Volume capabilities not supported")
	}
	// Validate volume size exists in spec
	if req.GetCapacityRange() == nil || req.GetCapacityRange().RequiredBytes == 0 {
		return status.Error(codes.InvalidArgument, "Volume size is a required parameter")
	}
	return nil
}

// ValidateDeleteVolumeRequest is the helper function to validate
// DeleteVolumeRequest for all block controllers.
// Function returns error if validation fails otherwise returns nil.
func ValidateDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	//check for required parameters
	if len(req.VolumeId) == 0 {
		msg := "Volume ID is a required parameter."
		klog.Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	return nil
}

// CheckAPI checks if specified version is 6.7.3 or higher
func CheckAPI(version string) error {
	items := strings.Split(version, ".")
	if len(items) <= 2 || len(items) > 3 {
		return fmt.Errorf("Invalid API Version format")
	}
	major, err := strconv.Atoi(items[0])
	if err != nil {
		return fmt.Errorf("Invalid Major Version value")
	}
	// If major version is 7 or above, return nil
	if major > MinSupportedVCenterMajor {
		return nil
	}
	// If major version is 6, should be 6.7.3 or higher
	if len(items) != 3 {
		return fmt.Errorf("Invalid API Version format")
	}
	minor, err := strconv.Atoi(items[1])
	if err != nil {
		return fmt.Errorf("Invalid Minor Version value")
	}
	patch, err := strconv.Atoi(items[2])
	if err != nil {
		return fmt.Errorf("Invalid Patch Version value")
	}

	if major < MinSupportedVCenterMajor || (major == MinSupportedVCenterMajor && minor < MinSupportedVCenterMinor) ||
		(major == MinSupportedVCenterMajor && minor == MinSupportedVCenterMinor && patch < MinSupportedVCenterPatch) {
		return fmt.Errorf("The minimum supported vCenter is 6.7.3")
	}

	return nil
}
