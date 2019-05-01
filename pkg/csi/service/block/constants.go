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

const (
	// MbInBytes is the number of bytes in one mebibyte.
	MbInBytes = int64(1024 * 1024)

	// GbInBytes is the number of bytes in one gibibyte.
	GbInBytes = int64(1024 * 1024 * 1024)

	// DefaultGbDiskSize is the default disk size in gibibytes.
	DefaultGbDiskSize = int64(10)
	// FirstClassDiskTypeString in string form
	DiskTypeString = "vSphere CNS Block Volume"

	// AttributeFirstClassDiskType is a Kubernetes volume label.
	AttributeDiskType = "type"
	// AttributeDiskParentType is a Kubernetes volume label.
	AttributeDiskParentType = "parent_type"
	// AttributeDiskParentName is a Kubernetes volume label.
	AttributeDiskParentName = "parent_name"
	// AttributeStoragePolicyType is a Kubernetes volume label.
	AttributeStoragePolicyType = "policy_type"
	// AttributeStoragePolicyType is a Kubernetes volume label.
	AttributeStoragePolicyName = "policy_name"

	// DatastoreType is the permitted value for AttributeDiskParentType
	DatastoreType = "Datastore"
	//StoragePolicyType is the permitted value for AtrributeStoragePolicyType
	StoragePolicyType = "StoragePolicy"

	//ProviderID Prefix
	ProviderPrefix = "vsphere://"
	// SCSI Disk Indetifier
	AttributeFirstClassDiskPage83Data = "page83data"
)
