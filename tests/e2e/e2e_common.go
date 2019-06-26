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

package e2e

import (
	"github.com/onsi/gomega"
	"os"
	"strconv"
	"time"
)

const (
	envSharedDatastoreURL                      = "SHARED_VSPHERE_DATASTORE_URL"
	envNonSharedStorageClassDatastoreURL       = "NONSHARED_VSPHERE_DATASTORE_URL"
	scParamDatastoreURL                        = "DatastoreURL"
	diskSize                                   = "2Gi"
	diskSizeInMb                               = int64(2048)
	e2evSphereCSIBlockDriverName               = "block.vsphere.csi.vmware.com"
	envVolumeOperationsScale                   = "VOLUME_OPS_SCALE"
	envStoragePolicyNameForSharedDatastores    = "STORAGE_POLICY_FOR_SHARED_DATASTORES"
	envStoragePolicyNameForNonSharedDatastores = "STORAGE_POLICY_FOR_NONSHARED_DATASTORES"
	scParamStoragePolicyName                   = "StoragePolicyName"
	poll                                       = 2 * time.Second
	pollTimeout                                = 5 * time.Minute
	pollTimeoutShort                           = 1 * time.Minute / 2
	envPandoraSyncWaitTime                     = "PANDORA_SYNC_WAIT_TIME"
	envFullSyncWaitTime                        = "FULL_SYNC_WAIT_TIME"
	defaultPandoraSyncWaitTime                 = 90
	defaultFullSyncWaitTime                    = 1800
	sleepTimeOut                               = 30
	vsanhealthServiceName                      = "vsan-health"
	zoneKey                                    = "failure-domain.beta.kubernetes.io/zone"
	regionKey                                  = "failure-domain.beta.kubernetes.io/region"
	envTopology                                = "TOPOLOGY_VALUES"
)

// GetAndExpectStringEnvVar parses a string from env variable
func GetAndExpectStringEnvVar(varName string) string {
	varValue := os.Getenv(varName)
	gomega.Expect(varValue).NotTo(gomega.BeEmpty(), "ENV "+varName+" is not set")
	return varValue
}

// GetAndExpectIntEnvVar parses an int from env variable
func GetAndExpectIntEnvVar(varName string) int {
	varValue := GetAndExpectStringEnvVar(varName)
	varIntValue, err := strconv.Atoi(varValue)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Error Parsing "+varName)
	return varIntValue
}
