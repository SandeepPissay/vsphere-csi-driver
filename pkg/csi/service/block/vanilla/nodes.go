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
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	cnsnode "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/node"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
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

func (nodes *Nodes) GetNodeByName(nodeName string) (*vsphere.VirtualMachine, error) {
	return nodes.cnsNodeManager.GetNodeByName(nodeName)
}

// GetSharedDatastoresInK8SCluster returns list of DatastoreInfo objects for datastores accessible to all
// kubernetes nodes in the cluster.
func (nodes *Nodes) GetSharedDatastoresInK8SCluster(ctx context.Context) ([]*vsphere.DatastoreInfo, error) {
	nodeVMs, err := nodes.cnsNodeManager.GetAllNodes()
	if err != nil {
		klog.Errorf("Failed to get Nodes from nodeManager with err %+v", err)
		return nil, err
	}
	if len(nodeVMs) == 0 {
		errMsg := fmt.Sprintf("Empty List of Node VMs returned from nodeManager")
		klog.Errorf(errMsg)
		return make([]*vsphere.DatastoreInfo, 0), fmt.Errorf(errMsg)
	}
	var sharedDatastores []*vsphere.DatastoreInfo
	for _, nodeVM := range nodeVMs {
		klog.V(4).Infof("Getting accessible datastores for node %s", nodeVM.VirtualMachine)
		accessibleDatastores, err := nodeVM.GetAllAccessibleDatastores(ctx)
		if err != nil {
			return nil, err
		}
		if len(sharedDatastores) == 0 {
			sharedDatastores = accessibleDatastores
		} else {
			var sharedAccessibleDatastores []*vsphere.DatastoreInfo
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
			return nil, fmt.Errorf("No shared datastores found in the Kubernetes cluster for nodeVm: %+v", nodeVM)
		}
	}
	klog.V(3).Infof("sharedDatastores : %+v", sharedDatastores)
	return sharedDatastores, nil
}
