/*
Copyright 2020 The Kubernetes Authors.

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
package ov

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vslm"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/client"
	csitypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
)

type OrphanVolumeRequest struct {
	KubeConfigFile string
	VcUser         string
	VcPwd          string
	VcHost         string
	Datacenter     string
	Datastores     []string
}

type FcdInfo struct {
	FcdId     string
	Datastore string
	PvName    string
	IsOrphan  bool
}

type OrphanVolumeResult struct {
	Fcds []FcdInfo
}

// GetOrphanVolumes provides the point in time orphan volumes that exists in FCD but used in Kubernetes cluster.
func GetOrphanVolumes(ctx context.Context, req *OrphanVolumeRequest) (*OrphanVolumeResult, error) {
	config, err := clientcmd.BuildConfigFromFlags("", req.KubeConfigFile)
	if err != nil {
		fmt.Printf("BuildConfigFromFlags failed %v\n", err)
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("KubeClient creation failed %v\n", err)
		return nil, err
	}
	vcClient, err := client.GetClient(ctx, req.VcUser, req.VcPwd, req.VcHost)
	if err != nil {
		return nil, err
	}
	return GetOrphanVolumesWithClients(ctx, kubeClient, vcClient, req.Datacenter, req.Datastores)
}

func GetOrphanVolumesWithClients(ctx context.Context, kubeClient kubernetes.Interface, vcClient *govmomi.Client, dc string, dss []string) (*OrphanVolumeResult, error) {
	res := &OrphanVolumeResult{
		Fcds: make([]FcdInfo, 0),
	}

	volumeHandleToPvMap := make(map[string]string)
	fmt.Printf("Listing all PVs in the Kubernetes cluster...")
	pvs, err := kubeClient.CoreV1().PersistentVolumes().List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Found %d PVs in the Kubernetes cluster\n", len(pvs.Items))
	for _, pv := range pvs.Items {
		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == csitypes.Name {
			volumeHandleToPvMap[pv.Spec.CSI.VolumeHandle] = pv.Name
		}
	}

	finder := find.NewFinder(vcClient.Client, false)
	dcObj, err := finder.Datacenter(ctx, dc)
	if err != nil {
		fmt.Printf("Unable to find datacenter: %s\n", dc)
		return nil, err
	}
	m := vslm.NewObjectManager(vcClient.Client)
	finder.SetDatacenter(dcObj)
	for _, ds := range dss {
		fmt.Printf("Listing FCDs under datastore: %s\n", ds)
		dsObj, err := finder.Datastore(ctx, ds)
		if err != nil {
			fmt.Printf("Unable to find datastore: %s\n", ds)
			return nil, err
		}
		fcds, err := m.List(ctx, dsObj)
		if err != nil {
			fmt.Printf("Failed to list FCDs in datastore: %s\n", ds)
			return nil, err
		}
		fmt.Printf("Found %d FCDs under datastore: %s\n", len(fcds), ds)
		for _, fcd := range fcds {
			if pv, ok := volumeHandleToPvMap[fcd.Id]; !ok {
				res.Fcds = append(res.Fcds, FcdInfo{
					FcdId:     fcd.Id,
					Datastore: ds,
					IsOrphan:  true,
				})
			} else {
				res.Fcds = append(res.Fcds, FcdInfo{
					FcdId:     fcd.Id,
					Datastore: ds,
					PvName:    pv,
					IsOrphan:  false,
				})
			}
		}
	}
	return res, nil
}
