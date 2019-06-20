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
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	cnsnode "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/node"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	csitypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

type Nodes struct {
	cnsNodeManager cnsnode.Manager
	informMgr      *k8s.InformerManager
}

func (nodes *Nodes) Initialize(serviceAccount string) error {
	nodes.cnsNodeManager = cnsnode.GetManager()
	// Create the kubernetes client
	k8sclient, err := k8s.NewClient(serviceAccount)
	if err != nil {
		klog.Errorf("Creating Kubernetes client failed. Err: %v", err)
		return err
	}
	k8snodes, err := k8sclient.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		msg := fmt.Sprintf("Failed to get kubernetes nodes. Err: %v", err)
		klog.Error(msg)
		return err
	}
	for idx := range k8snodes.Items {
		node := &k8snodes.Items[idx]
		err := nodes.registerNode(node)
		if err != nil {
			klog.Errorf("Failed to register node:%q. err=%v", node.Name, err)
			return err
		}
	}
	nodes.informMgr = k8s.NewInformer(k8sclient)
	nodes.informMgr.AddNodeListener(nodes.nodeAdd, nil, nodes.nodeDelete)
	nodes.informMgr.Listen()
	return nil
}

func (nodes *Nodes) registerNode(node *v1.Node) error {
	nodeUUID := block.GetUUIDFromProviderID(node.Spec.ProviderID)
	err := nodes.cnsNodeManager.RegisterNode(nodeUUID, node.Name, node.GetObjectMeta())
	return err
}

func (nodes *Nodes) nodeAdd(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if node == nil || !ok {
		klog.Warningf("nodeAdd: unrecognized object %+v", obj)
		return
	}
	err := nodes.cnsNodeManager.RegisterNode(block.GetUUIDFromProviderID(node.Spec.ProviderID), node.Name, node.GetObjectMeta())
	if err != nil {
		klog.Warningf("Failed to register node:%q. err=%v", node.Name, err)
	}
}

func (nodes *Nodes) nodeDelete(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if node == nil || !ok {
		klog.Warningf("nodeDelete: unrecognized object %+v", obj)
		return
	}
	err := nodes.cnsNodeManager.UnregisterNode(node.Name)
	if err != nil {
		klog.Warningf("Failed to unregister node:%q. err=%v", node.Name, err)
	}
}

func (nodes *Nodes) GetNodeByName(nodeName string) (*cnsvsphere.VirtualMachine, error) {
	return nodes.cnsNodeManager.GetNodeByName(nodeName)
}

// GetSharedDatastoresInTopology returns shared accessible datastores for specified topologyRequirement and
// volumeAccessibleTopology containing zone and region where volume needs be provisioned
func (nodes *Nodes) GetSharedDatastoresInTopology(ctx context.Context, topologyRequirement *csi.TopologyRequirement, zoneCategoryName string, regionCategoryName string) ([]*cnsvsphere.DatastoreInfo, map[string]string, error) {
	klog.V(4).Infof("GetSharedDatastoresInTopology: called with topologyRequirement: %+v, zoneCategoryName: %s, regionCategoryName: %s", topologyRequirement, zoneCategoryName, regionCategoryName)
	nodeVMs, err := nodes.cnsNodeManager.GetAllNodes()
	if err != nil {
		klog.Errorf("Failed to get Nodes from nodeManager with err %+v", err)
		return nil, nil, err
	}
	if len(nodeVMs) == 0 {
		errMsg := fmt.Sprintf("Empty List of Node VMs returned from nodeManager")
		klog.Errorf(errMsg)
		return nil, nil, fmt.Errorf(errMsg)
	}

	// getNodesInZoneRegion takes zone and region as parameter and returns list of node VMs which belongs to specified
	// zone and region.
	getNodesInZoneRegion := func(zoneValue string, regionValue string) ([]*cnsvsphere.VirtualMachine, error) {
		klog.V(4).Infof("getNodesInZoneRegion: called with zoneValue: %s, regionValue: %s", zoneValue, regionValue)
		var nodeVMsInZoneAndRegion []*cnsvsphere.VirtualMachine
		for _, nodeVM := range nodeVMs {
			isNodeInZoneRegion, err := nodeVM.IsInZoneRegion(ctx, zoneCategoryName, regionCategoryName, zoneValue, regionValue)
			if err != nil {
				klog.Errorf("Failed to get zone/region for node VM: %q. err: %+v", nodeVM.InventoryPath, err)
				return nil, err
			}
			if isNodeInZoneRegion {
				nodeVMsInZoneAndRegion = append(nodeVMsInZoneAndRegion, nodeVM)
			}
		}
		return nodeVMsInZoneAndRegion, nil
	}

	// getSharedDatastoresInZoneRegion returns list of shared datastores for requested zone and region.
	getSharedDatastoresInZoneRegion := func(topologyArr []*csi.Topology) ([]*cnsvsphere.DatastoreInfo, map[string]string, error) {
		klog.V(4).Infof("getSharedDatastoresInZoneRegion: called with topologyArr: %+v", topologyArr)
		var nodeVMsInZoneAndRegion []*cnsvsphere.VirtualMachine
		var sharedDatastores []*cnsvsphere.DatastoreInfo
		var volumeAccessibleTopology = make(map[string]string)
		for _, topology := range topologyArr {
			segments := topology.GetSegments()
			zone := segments[csitypes.LabelZoneFailureDomain]
			region := segments[csitypes.LabelRegionFailureDomain]
			klog.V(4).Info(fmt.Sprintf("getting shared datastores for zone [%s] and region [%s]", zone, region))
			nodeVMsInZoneAndRegion, err = getNodesInZoneRegion(zone, region)
			if err != nil {
				klog.Errorf("Failed to find Nodes in the zone: [%s] and region: [%s]. Error: %+v", zone, region, err)
				return nil, nil, err
			}
			sharedDatastores, err = nodes.GetSharedDatastoresForVMs(ctx, nodeVMsInZoneAndRegion)
			if err != nil {
				klog.Errorf("failed to get shared datastores for nodes: %+v. Error: %+v", nodeVMsInZoneAndRegion, err)
				return nil, nil, err
			}
			if len(sharedDatastores) > 0 {
				if zone != "" {
					volumeAccessibleTopology[csitypes.LabelZoneFailureDomain] = zone
				}
				if region != "" {
					volumeAccessibleTopology[csitypes.LabelRegionFailureDomain] = region
				}
				break
			}
		}
		klog.V(4).Infof("Obtained sharedDatastores : %+v for volumeAccessibleTopology: %+v", sharedDatastores, volumeAccessibleTopology)
		return sharedDatastores, volumeAccessibleTopology, nil
	}

	var sharedDatastores []*cnsvsphere.DatastoreInfo
	var volumeAccessibleTopology = make(map[string]string)
	if topologyRequirement != nil && topologyRequirement.GetPreferred() != nil {
		klog.V(3).Infoln("Using preferred topology")
		sharedDatastores, volumeAccessibleTopology, err = getSharedDatastoresInZoneRegion(topologyRequirement.GetPreferred())
		if err != nil {
			klog.Errorf("Failed to ")
			return nil, nil, err
		}
	}
	if len(sharedDatastores) == 0 && topologyRequirement != nil && topologyRequirement.GetRequisite() != nil {
		klog.V(3).Infoln("Using requisite topology")
		sharedDatastores, volumeAccessibleTopology, err = getSharedDatastoresInZoneRegion(topologyRequirement.GetRequisite())
	}
	return sharedDatastores, volumeAccessibleTopology, nil
}

// GetSharedDatastoresInK8SCluster returns list of DatastoreInfo objects for datastores accessible to all
// kubernetes nodes in the cluster.
func (nodes *Nodes) GetSharedDatastoresInK8SCluster(ctx context.Context) ([]*cnsvsphere.DatastoreInfo, error) {
	nodeVMs, err := nodes.cnsNodeManager.GetAllNodes()
	if err != nil {
		klog.Errorf("Failed to get Nodes from nodeManager with err %+v", err)
		return nil, err
	}
	if len(nodeVMs) == 0 {
		errMsg := fmt.Sprintf("Empty List of Node VMs returned from nodeManager")
		klog.Errorf(errMsg)
		return make([]*cnsvsphere.DatastoreInfo, 0), fmt.Errorf(errMsg)
	}
	sharedDatastores, err := nodes.GetSharedDatastoresForVMs(ctx, nodeVMs)
	if err != nil {
		klog.Errorf("Failed to get shared datastores for node VMs. Err: %+v", err)
		return nil, err
	}
	klog.V(3).Infof("sharedDatastores : %+v", sharedDatastores)
	return sharedDatastores, nil
}

// GetSharedDatastoresForVMs returns shared datastores accessible to specified nodeVMs list
func (nodes *Nodes) GetSharedDatastoresForVMs(ctx context.Context, nodeVMs []*cnsvsphere.VirtualMachine) ([]*cnsvsphere.DatastoreInfo, error) {
	var sharedDatastores []*cnsvsphere.DatastoreInfo
	for _, nodeVM := range nodeVMs {
		klog.V(4).Infof("Getting accessible datastores for node %s", nodeVM.VirtualMachine)
		accessibleDatastores, err := nodeVM.GetAllAccessibleDatastores(ctx)
		if err != nil {
			return nil, err
		}
		if len(sharedDatastores) == 0 {
			sharedDatastores = accessibleDatastores
		} else {
			var sharedAccessibleDatastores []*cnsvsphere.DatastoreInfo
			for _, sharedDs := range sharedDatastores {
				// Check if sharedDatastores is found in accessibleDatastores
				for _, accessibleDs := range accessibleDatastores {
					// Intersection is performed based on the datastoreUrl as this uniquely identifies the datastore.
					if sharedDs.Info.Url == accessibleDs.Info.Url {
						sharedAccessibleDatastores = append(sharedAccessibleDatastores, sharedDs)
						break
					}
				}
			}
			sharedDatastores = sharedAccessibleDatastores
		}
		if len(sharedDatastores) == 0 {
			return nil, fmt.Errorf("No shared datastores found for nodeVm: %+v", nodeVM)
		}
	}
	return sharedDatastores, nil
}
