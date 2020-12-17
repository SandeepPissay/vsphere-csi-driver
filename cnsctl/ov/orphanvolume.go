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
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vslm"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/vm"
	csitypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	"time"
)

type OrphanVolumeRequest struct {
	KubeConfigFile string
	VcClient       *govmomi.Client
	Datacenter     string
	Datastores     []string
	LongListing    bool
}

type FcdInfo struct {
	FcdId          string
	Datastore      string
	PvName         string
	IsOrphan       bool
	CreateTime     time.Time
	CapacityInMB   int64
	IsAttached     bool
	AttachedVmName string
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
	return GetOrphanVolumesWithClients(ctx, kubeClient, req)
}

func GetOrphanVolumesWithClients(ctx context.Context, kubeClient kubernetes.Interface, req *OrphanVolumeRequest) (*OrphanVolumeResult, error) {
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

	finder := find.NewFinder(req.VcClient.Client, false)
	dcObj, err := finder.Datacenter(ctx, req.Datacenter)
	if err != nil {
		fmt.Printf("Unable to find datacenter: %s\n", req.Datacenter)
		return nil, err
	}
	m := vslm.NewObjectManager(req.VcClient.Client)
	finder.SetDatacenter(dcObj)
	for _, ds := range req.Datastores {
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

		fcdVmMap := make(map[string]string)
		if req.LongListing {
			retObjSpecs := make([]types.RetrieveVStorageObjSpec, 0, len(fcds))
			for _, fcd := range fcds {
				retObjSpecs = append(retObjSpecs, types.RetrieveVStorageObjSpec{
					Id:        fcd,
					Datastore: dsObj.Reference(),
				})
			}
			retObjAsso := &types.RetrieveVStorageObjectAssociations{
				This: m.Reference(),
				Ids:  retObjSpecs,
			}
			resObjAsso, err := methods.RetrieveVStorageObjectAssociations(ctx, req.VcClient.RoundTripper, retObjAsso)
			if err != nil {
				fmt.Printf("Failed to get VM associations for retObjAsso: %+v\n", retObjAsso)
				return nil, err
			}
			for _, res := range resObjAsso.Returnval {
				if res.Fault == nil && len(res.VmDiskAssociations) > 0 {
					vmMo, err := vm.GetVirtualMachine(ctx, req.VcClient.Client, res.VmDiskAssociations[0].VmId)
					if err != nil {
						fmt.Printf("FCD %s is attached to VM. Failed to get the VM. Err: %+v\n", res.Id.Id, err)
						return nil, err
					}
					fcdVmMap[res.Id.Id] = vmMo.Name
				}
			}
		}

		for _, fcd := range fcds {
			fcdInfo := FcdInfo{
				FcdId:     fcd.Id,
				Datastore: ds,
			}
			var vso *types.VStorageObject
			if req.LongListing {
				vso, err = m.Retrieve(ctx, dsObj, fcd.Id)
				if err != nil {
					fmt.Printf("Failed to retrieve VStorageObject for FCD: %s\n", fcd.Id)
					return nil, err
				}
				fcdInfo.CreateTime = vso.Config.CreateTime
				fcdInfo.CapacityInMB = vso.Config.CapacityInMB
				if vmName, ok := fcdVmMap[fcd.Id]; ok {
					fcdInfo.IsAttached = true
					fcdInfo.AttachedVmName = vmName
				}
			}
			if pv, ok := volumeHandleToPvMap[fcd.Id]; !ok {
				fcdInfo.IsOrphan = true
				res.Fcds = append(res.Fcds, fcdInfo)
			} else {
				fcdInfo.PvName = pv
				fcdInfo.IsOrphan = false
				res.Fcds = append(res.Fcds, fcdInfo)
			}
		}
	}
	return res, nil
}
