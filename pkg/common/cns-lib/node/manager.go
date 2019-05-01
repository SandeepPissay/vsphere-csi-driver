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

package node

import (
	"errors"
	"fmt"
	"sync"

	"k8s.io/klog"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
)

var (
	// ErrNodeNotFound is returned when a node isn't found.
	ErrNodeNotFound = errors.New("node wasn't found")
	// ErrNodeAlreadyRegistered is returned when registration is attempted for
	// a previously registered node.
	ErrNodeAlreadyRegistered = errors.New("node was already registered")
)

// Manager provides functionality to manage nodes.
type Manager interface {
	// RegisterNode registers a node given its UUID, name and Metadata.
	RegisterNode(nodeUUID string, nodeName string, metadata Metadata) error
	// DiscoverNode discovers a registered node given its UUID. This method
	// scans all virtual centers registered on the VirtualCenterManager for a
	// virtual machine with the given UUID.
	DiscoverNode(nodeUUID string) error
	// GetNodeMetadata returns Metadata for a registered node given its UUID.
	GetNodeMetadata(nodeUUID string) (Metadata, error)
	// GetAllNodeMetadata returns Metadata for all registered nodes.
	GetAllNodeMetadata() []Metadata
	// GetNode refreshes and returns the VirtualMachine for a registered node
	// given its UUID.
	GetNode(nodeUUID string) (*vsphere.VirtualMachine, error)
	// GetNodeByName refreshes and returns the VirtualMachine for a registered node
	// given its name.
	GetNodeByName(nodeName string) (*vsphere.VirtualMachine, error)
	// GetAllNodes refreshes and returns VirtualMachine for all registered
	// nodes. If nodes are added or removed concurrently, they may or may not be
	// reflected in the result of a call to this method.
	GetAllNodes() ([]*vsphere.VirtualMachine, error)
	// UnregisterNode unregisters a registered node given its name.
	UnregisterNode(nodeName string) error
	// GetNodeName returns node name for requested node UUID.
	GetNodeName(nodeUUID string) (string, error)
}

// Metadata represents node metadata.
type Metadata interface{}

var (
	// managerInstance is a Manager singleton.
	managerInstance *defaultManager
	// onceForManager is used for initializing the Manager singleton.
	onceForManager sync.Once
)

// GetManager returns the Manager singleton.
func GetManager() Manager {
	onceForManager.Do(func() {
		klog.V(1).Info("Initializing node.defaultManager...")
		managerInstance = &defaultManager{
			nodeVMs:      sync.Map{},
			nodeMetadata: sync.Map{},
		}
		klog.V(1).Info("node.defaultManager initialized")
	})
	return managerInstance
}

// defaultManager holds node information and provides functionality around it.
type defaultManager struct {
	// nodeVMs maps node UUIDs to VirtualMachine objects.
	nodeVMs sync.Map
	// nodeMetadata maps node UUIDs to generic metadata.
	nodeMetadata sync.Map
	// node name to node UUI map.
	nodeNameToUUID sync.Map
}

func (m *defaultManager) GetNodeName(nodeUUID string) (string, error) {
	var nodeName string
	m.nodeNameToUUID.Range(func(k, v interface{}) bool {
		if v.(string) == nodeUUID {
			nodeName = k.(string)
			return false
		}
		return true
	})
	if nodeName == "" {
		err := fmt.Errorf("unable to find node name for node uuid:%s", nodeUUID)
		return "", err
	}
	return nodeName, nil
}

func (m *defaultManager) RegisterNode(nodeUUID string, nodeName string, metadata Metadata) error {
	if _, exists := m.nodeMetadata.Load(nodeUUID); exists {
		klog.Errorf("Node already exists with nodeUUID %s and metadata %v,failed to register", nodeUUID, metadata)
		return ErrNodeAlreadyRegistered
	}

	m.nodeMetadata.Store(nodeUUID, metadata)
	m.nodeNameToUUID.Store(nodeName, nodeUUID)
	klog.V(2).Infof("Successfully registered node with nodeUUID %s and metadata %v", nodeUUID, metadata)
	return m.DiscoverNode(nodeUUID)
}

func (m *defaultManager) DiscoverNode(nodeUUID string) error {
	if _, err := m.GetNodeMetadata(nodeUUID); err != nil {
		klog.Errorf("Node wasn't found with nodeUUID %s, failed to discover with err: %v", nodeUUID, err)
		return err
	}

	vm, err := vsphere.GetVirtualMachineByUUID(nodeUUID, false)
	if err != nil {
		klog.Errorf("Couldn't find VM instance with nodeUUID %s, failed to discover with err: %v", nodeUUID, err)
		return err
	}

	m.nodeVMs.Store(nodeUUID, vm)
	klog.V(2).Infof("Successfully discovered node with nodeUUID %s in vm %v", nodeUUID, vm)
	return nil
}

func (m *defaultManager) GetNodeMetadata(nodeUUID string) (Metadata, error) {
	if metadata, ok := m.nodeMetadata.Load(nodeUUID); ok {
		klog.V(2).Infof("Node metadata was found with nodeUUID %s and metadata %v", nodeUUID, metadata)
		return metadata, nil
	}
	klog.Errorf("Node metadata wasn't found with nodeUUID %s", nodeUUID)
	return nil, ErrNodeNotFound
}

func (m *defaultManager) GetAllNodeMetadata() []Metadata {
	var nodeMetadata []Metadata
	m.nodeMetadata.Range(func(_, metadataInf interface{}) bool {
		nodeMetadata = append(nodeMetadata, metadataInf.(Metadata))
		return true
	})
	return nodeMetadata
}

func (m *defaultManager) GetNodeByName(nodeName string) (*vsphere.VirtualMachine, error) {
	nodeUUID, found := m.nodeNameToUUID.Load(nodeName)
	if !found {
		klog.Errorf("Node not found with nodeName %s", nodeName)
		return nil, ErrNodeNotFound
	}
	return m.GetNode(nodeUUID.(string))
}
func (m *defaultManager) GetNode(nodeUUID string) (*vsphere.VirtualMachine, error) {
	vmInf, discovered := m.nodeVMs.Load(nodeUUID)
	if !discovered {
		klog.V(2).Infof("Node hasn't been discovered yet with nodeUUID %s", nodeUUID)

		if err := m.DiscoverNode(nodeUUID); err != nil {
			klog.Errorf("Failed to discover node with nodeUUID %s with err: %v", nodeUUID, err)
			return nil, err
		}

		vmInf, _ = m.nodeVMs.Load(nodeUUID)
		klog.V(2).Infof("Node was successfully discovered with nodeUUID %s in vm %v", nodeUUID, vmInf)

		return vmInf.(*vsphere.VirtualMachine), nil
	}

	vm := vmInf.(*vsphere.VirtualMachine)
	klog.V(1).Infof("Renewing virtual machine %v with nodeUUID %s", vm, nodeUUID)

	if err := vm.Renew(true); err != nil {
		klog.Errorf("Failed to renew VM %v with nodeUUID %s with err: %v", vm, nodeUUID, err)
		return nil, err
	}

	klog.V(1).Infof("VM %v was successfully renewed with nodeUUID %s", vm, nodeUUID)
	return vm, nil
}

func (m *defaultManager) GetAllNodes() ([]*vsphere.VirtualMachine, error) {
	var vms []*vsphere.VirtualMachine
	var err error
	reconnectedHosts := make(map[string]bool)

	m.nodeVMs.Range(func(nodeUUIDInf, vmInf interface{}) bool {
		// If an entry was concurrently deleted from vm, Range could
		// possibly return a nil value for that key.
		// See https://golang.org/pkg/sync/#Map.Range for more info.
		if vmInf == nil {
			klog.Warningf("VM instance was nil, ignoring with nodeUUID %v", nodeUUIDInf)
			return true
		}

		nodeUUID := nodeUUIDInf.(string)
		vm := vmInf.(*vsphere.VirtualMachine)

		if reconnectedHosts[vm.VirtualCenterHost] {
			klog.V(2).Infof("Renewing VM %v, no new connection needed: nodeUUID %s", vm, nodeUUID)
			err = vm.Renew(false)
		} else {
			klog.V(2).Infof("Renewing VM %v with new connection: nodeUUID %s", vm, nodeUUID)
			err = vm.Renew(true)
			reconnectedHosts[vm.VirtualCenterHost] = true
		}

		if err != nil {
			klog.Errorf("Failed to renew VM %v with nodeUUID %s, aborting get all nodes", vm, nodeUUID)
			return false
		}

		klog.V(1).Infof("Updated VM %v for node with nodeUUID %s", vm, nodeUUID)
		vms = append(vms, vm)
		return true
	})

	if err != nil {
		return nil, err
	}
	return vms, nil
}

func (m *defaultManager) UnregisterNode(nodeName string) error {
	nodeUUID, found := m.nodeNameToUUID.Load(nodeName)
	if !found {
		klog.Errorf("Node not found with nodeName %s", nodeName)
		return ErrNodeNotFound
	}
	if _, err := m.GetNodeMetadata(nodeUUID.(string)); err != nil {
		klog.Errorf("Node wasn't found, failed to unregister with nodeUUID %s with err: %v", nodeUUID, err)
		return err
	}
	m.nodeNameToUUID.Delete(nodeName)
	m.nodeMetadata.Delete(nodeUUID)
	m.nodeVMs.Delete(nodeUUID)
	klog.V(2).Infof("Successfully unregistered node with nodeName %s", nodeName)
	return nil
}
