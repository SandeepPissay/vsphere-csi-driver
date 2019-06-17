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

package wcp

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	"strings"
)

// ValidateCreateVolumeRequest is the helper function to validate
// CreateVolumeRequest for WCP CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateWCPCreateVolumeRequest(req *csi.CreateVolumeRequest) error {
	// Get create params
	params := req.GetParameters()
	for paramName := range params {
		paramName = strings.ToLower(paramName)
		if paramName != block.AttributeStoragePolicyID && paramName != block.AttributeFsType {
			msg := fmt.Sprintf("Volume parameter %s is not a valid WCP CSI parameter.", paramName)
			return status.Error(codes.InvalidArgument, msg)
		}
	}
	return block.ValidateCreateVolumeRequest(req)
}

// validateWCPDeleteVolumeRequest is the helper function to validate
// DeleteVolumeRequest for WCP CSI driver.
// Function returns error if validation fails otherwise returns nil.
func validateWCPDeleteVolumeRequest(req *csi.DeleteVolumeRequest) error {
	return block.ValidateDeleteVolumeRequest(req)
}
