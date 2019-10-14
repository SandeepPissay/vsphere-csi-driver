/*
Copyright 2018 The Kubernetes Authors.

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

package types

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
)

// CnsController is the interface for the CSI Controller Server plus extra methods
// required to support CNS API backend
type CnsController interface {
	csi.ControllerServer
	Init(config *config.Config) error
}

// ClusterFlavor represents the allowed strings for the env variable CLUSTER_FLAVOR
// Allowed constants are defined in constants.go
type ClusterFlavor string