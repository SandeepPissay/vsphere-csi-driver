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

	// DiskTypeString is the value for the PersistentVolume's attribute "type"
	DiskTypeString = "vSphere CNS Block Volume"

	// AttributeDiskType is a PersistentVolume's attribute.
	AttributeDiskType = "type"

	// AttributeDiskParentType is the type of the PersistentVolume's parameters.
	// For Example: parent_type: "Datastore"
	AttributeDiskParentType = "parent_type"

	// AttributeDiskParentName is the name for the PersistentVolume's parameter specified with parent_type.
	// For Example: parent_name: "sharedVmfs-0"
	AttributeDiskParentName = "parent_name"
	// AttributeStoragePolicyType is a Kubernetes volume label.
	AttributeStoragePolicyType = "policy_type"
	// AttributeStoragePolicyType is a Kubernetes volume label.
	AttributeStoragePolicyName = "policy_name"

	// DatastoreType is the permitted value for AttributeDiskParentType
	DatastoreType = "Datastore"
	//StoragePolicyType is the permitted value for AtrributeStoragePolicyType
	StoragePolicyType = "StoragePolicy"

	//ProviderPrefix is the prefix used for the ProviderID set on the node
	// Example: vsphere://4201794a-f26b-8914-d95a-edeb7ecc4a8f
	ProviderPrefix = "vsphere://"

	// AttributeFirstClassDiskPage83Data is the SCSI Disk Identifier
	AttributeFirstClassDiskPage83Data = "page83data"

	// BlockVolumeType is the VolumeType for CNS Volume
	BlockVolumeType = "BLOCK"
)
