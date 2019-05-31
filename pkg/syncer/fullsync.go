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

package syncer

import (
	"github.com/davecgh/go-spew/spew"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	volumes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	"sync"
)

const (
	createOperation = "Create"
	updateOperation = "Update"
	vSphereCSIDriverName = "block.vsphere.csi.vmware.com"
)

type pvcMap = map[string]*v1.PersistentVolumeClaim
type podMap = map[string]*v1.Pod

// triggerFullSync triggers full sync
func triggerFullSync(k8sclient clientset.Interface, metadataSyncer *metadataSyncInformer) {
	klog.V(2).Infof("CSPFullSync: start")
	// Get all PVs from kubernetes
	allPVs, err := k8sclient.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		klog.Warningf("CSPFullSync: Failed to get PVs from kubernetes. Err: %v", err)
		return
	}

	// Get PVs in State "Bound", "Available" or "Relesased"
	k8sPVs := getPVsInBoundAvailableOrReleased(allPVs)

	// pvToPVCMap maps pv name to corresponding PVC
	// pvcToPodMap maps pvc to the mounted Pod
	pvToPVCMap, pvcToPodMap, err := buildPVCMapPodMap(k8sclient, k8sPVs)
	if (err != nil) {
		// Failed to build map, cannot do fullsync
		return
	}

	//Call CNS QueryAll to get container volumes by cluster ID
	queryFilter := cnstypes.CnsQueryFilter{
		ContainerClusterIds: []string{
			metadataSyncer.cfg.Global.ClusterID,
		},
	}
	querySelection := cnstypes.CnsQuerySelection{}
	queryAllResult, err := volumes.GetManager(metadataSyncer.vcenter).QueryAllVolume(queryFilter, querySelection)
	if err != nil {
		klog.Warningf("CSPFullSync:failed to queryAllVolume with err %v", err)
		return
	}
	cnsVolumeArray := queryAllResult.Volumes

	k8sPVsMap := buildVolumeMap(k8sPVs, cnsVolumeArray, pvToPVCMap, pvcToPodMap, metadataSyncer)

	volToBeCreated, volToBeUpdated := identifyVolumesToBeCreatedUpdated(k8sPVs, k8sPVsMap)
	createSpecArray := constructCnsCreateSpec(volToBeCreated, pvToPVCMap, pvcToPodMap, metadataSyncer)
	updateSpecArray := constructCnsUpdateSpec(volToBeUpdated, pvToPVCMap, pvcToPodMap, metadataSyncer)

	volToBeDeleted := identifyVolumesToBeDeleted(cnsVolumeArray, k8sPVsMap)

	wg := sync.WaitGroup{}
	wg.Add(3)
	go fullSyncCreateVolumes(createSpecArray, metadataSyncer, &wg)
	go fullSyncDeleteVolumes(volToBeDeleted, metadataSyncer, &wg)
	go fullSyncUpdateVolumes(updateSpecArray, metadataSyncer, &wg)
	wg.Wait()
	klog.V(2).Infof("CSPFullSync: end")
}

// getPVsInBoundAvailableOrReleased return PVs in Bound, Available or Released state
func getPVsInBoundAvailableOrReleased(pvList *v1.PersistentVolumeList) []*v1.PersistentVolume {
	var pvsInDesiredState []*v1.PersistentVolume
	pvsInDesiredState = []*v1.PersistentVolume{}
	for _, pv := range pvList.Items {
		if pv.Spec.CSI.Driver == vSphereCSIDriverName {
			klog.V(4).Infof("CSPFullSync: pv %v is in state %v", pv.Spec.CSI.VolumeHandle, pv.Status.Phase)
			if pv.Status.Phase == v1.VolumeBound || pv.Status.Phase == v1.VolumeAvailable || pv.Status.Phase == v1.VolumeReleased {
				pvsInDesiredState = append(pvsInDesiredState, &pv)
			}
		}
	}
	return pvsInDesiredState
}

// fullSyncCreateVolumes create volumes with given array of createSpec
func fullSyncCreateVolumes(createSpecArray []cnstypes.CnsVolumeCreateSpec, metadataSyncer *metadataSyncInformer, wg *sync.WaitGroup) {
	for _, createSpec := range createSpecArray {
		klog.V(4).Infof("CSPFullSync: Calling CreateVolume for volume  %s with create spec %+v", createSpec.Name, spew.Sdump(createSpec))
		_, err := volumes.GetManager(metadataSyncer.vcenter).CreateVolume(&createSpec)

		if err != nil {
			klog.Warningf("CSPFullSync: Failed to create disk %s with error %+v", createSpec.Name, err)
		}
	}
	wg.Done()
}

// fullSyncDeleteVolumes delete volumes with given array of volumeId
func fullSyncDeleteVolumes(volumeIDDeleteArray []cnstypes.CnsVolumeId, metadataSyncer *metadataSyncInformer, wg *sync.WaitGroup) {
	deleteDisk := false
	for _, volID := range volumeIDDeleteArray {
		klog.V(4).Infof("CSPFullSync: Calling DeleteVolume for volume %v with delete disk %v", volID, deleteDisk)
		err := volumes.GetManager(metadataSyncer.vcenter).DeleteVolume(volID.Id, deleteDisk)
		if err != nil {
			klog.Warningf("CSPFullSync: Failed to delete volume %s with error %+v", volID, err)
		}
	}
	wg.Done()
}

// fullSyncUpdateVolumes update metadata for volumes with given array of createSpec
func fullSyncUpdateVolumes(updateSpecArray []cnstypes.CnsVolumeMetadataUpdateSpec, metadataSyncer *metadataSyncInformer, wg *sync.WaitGroup) {
	for _, updateSpec := range updateSpecArray {
		klog.V(4).Infof("CSPFullSync: Calling UpdateVolumeMetadata for volume %s with updateSpec: %+v", updateSpec.VolumeId.Id, spew.Sdump(updateSpec))
		if err := volumes.GetManager(metadataSyncer.vcenter).UpdateVolumeMetadata(&updateSpec); err != nil {
			klog.Warningf("CSPFullSync:UpdateVolumeMetadata failed with err %v", err)
		}
	}
	wg.Done()
}

// buildMetadataList build metadata list for given PV
// metadata list may include PV metadata, PVC metadata and POD metadata
func buildMetadataList(pv *v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap) []cnstypes.BaseCnsEntityMetadata {
	var metadataList []cnstypes.BaseCnsEntityMetadata

	// get pv metadata
	pvMetadata := cnsvsphere.GetCnsKubernetesEntityMetaData(pv.Name, pv.GetLabels(), false, string(cnstypes.CnsKubernetesEntityTypePV), pv.Namespace)
	metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvMetadata))
	if pvc, ok := pvToPVCMap[pv.Name]; ok {
		// get pvc metadata
		pvcMetadata := cnsvsphere.GetCnsKubernetesEntityMetaData(pvc.Name, pvc.GetLabels(), false, string(cnstypes.CnsKubernetesEntityTypePVC), pvc.Namespace)
		metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvcMetadata))

		key := pvc.Namespace + "/" + pvc.Name
		if pod, ok := pvcToPodMap[key]; ok {
			// get pod metadata
			podMetadata := cnsvsphere.GetCnsKubernetesEntityMetaData(pod.Name, nil, false, string(cnstypes.CnsKubernetesEntityTypePOD), pod.Namespace)
			metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(podMetadata))
		}
	}
	klog.V(4).Infof("CSPFullSync: buildMetadataList=%+v \n", spew.Sdump(metadataList))
	return metadataList
}

// buildVolumeMap build k8sPVMap which maps volume id to a string "Create"/"Update" to indicate the PV need to be
// created/updated in CNS cache
func buildVolumeMap(pvList []*v1.PersistentVolume, cnsVolumeList []cnstypes.CnsVolume, pvToPVCMap pvcMap, pvcToPodMap podMap, metadataSyncer *metadataSyncInformer) map[string]string {
	k8sPVMap := make(map[string]string)
	cnsVolumeMap := make(map[string]bool)

	for _, vol := range cnsVolumeList {
		cnsVolumeMap[vol.VolumeId.Id] = true
	}

	for _, pv := range pvList {
		k8sPVMap[pv.Spec.CSI.VolumeHandle] = ""
		if cnsVolumeMap[pv.Spec.CSI.VolumeHandle] {
			// PV exist in both K8S and CNS cache, check metadata has been changed or not
			queryFilter := cnstypes.CnsQueryFilter{
				VolumeIds: []cnstypes.CnsVolumeId{
					cnstypes.CnsVolumeId{
						Id: pv.Spec.CSI.VolumeHandle,
					},
				},
			}

			queryResult, err := volumes.GetManager(metadataSyncer.vcenter).QueryVolume(queryFilter)
			if err == nil && queryResult != nil && len(queryResult.Volumes) > 0 {
				if (&queryResult.Volumes[0].Metadata != nil) {
					cnsMetadata := queryResult.Volumes[0].Metadata.EntityMetadata
					metadataList := buildMetadataList(pv, pvToPVCMap, pvcToPodMap)
					if !cnsvsphere.CompareK8sandCNSVolumeMetadata(metadataList, cnsMetadata) {
						k8sPVMap[pv.Spec.CSI.VolumeHandle] = updateOperation
					}
				} else {
					// metadata does not exist in CNS cache even the volume has an entry in CNS cache
					klog.Warningf("CSPFullSync:No metadata found for volume %v", pv.Spec.CSI.VolumeHandle)
					k8sPVMap[pv.Spec.CSI.VolumeHandle] = updateOperation
				}
			}
		} else {
			// PV exist in K8S but not in CNS cache, need to create
			k8sPVMap[pv.Spec.CSI.VolumeHandle] = createOperation
		}
	}

	return k8sPVMap
}

// identifyVolumesToBeCreatedUpdated return list of PV need to be created and updated
func identifyVolumesToBeCreatedUpdated(pvList []*v1.PersistentVolume, k8sPVMap map[string]string) ([]*v1.PersistentVolume, []*v1.PersistentVolume) {
	pvToBeCreated := []*v1.PersistentVolume{}
	pvToBeUpdated := []*v1.PersistentVolume{}
	for _, pv := range pvList {
		if k8sPVMap[pv.Spec.CSI.VolumeHandle] == createOperation {
			pvToBeCreated = append(pvToBeCreated, pv)
		} else if k8sPVMap[pv.Spec.CSI.VolumeHandle] == updateOperation {
			pvToBeUpdated = append(pvToBeUpdated, pv)
		}
	}

	return pvToBeCreated,pvToBeUpdated
}

// identifyVolumesToBeDeleted return list of volumeId need to be deleted
func identifyVolumesToBeDeleted(cnsVolumeList []cnstypes.CnsVolume, k8sPVMap map[string]string) []cnstypes.CnsVolumeId {
	var volToBeDeleted []cnstypes.CnsVolumeId
	for _, vol := range cnsVolumeList {
		if _, ok := k8sPVMap[vol.VolumeId.Id]; !ok {
			volToBeDeleted = append(volToBeDeleted, vol.VolumeId)
		}
	}
	return volToBeDeleted
}

// constructCnsCreateSpec construct CnsVolumeCreateSpec for given list of PVs
func constructCnsCreateSpec(pvList []*v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeCreateSpec {
	var createSpecArray []cnstypes.CnsVolumeCreateSpec
	for _, pv := range pvList {
		// Create new metadata spec
		metadataList := buildMetadataList(pv, pvToPVCMap, pvcToPodMap)
		// volume exist in K8S, but not in CNS cache, need to create this volume
		createSpec := cnstypes.CnsVolumeCreateSpec{
			Name:       pv.Name,
			VolumeType: block.BlockVolumeType,
			Metadata: cnstypes.CnsVolumeMetadata{
				ContainerCluster: cnsvsphere.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
				EntityMetadata:   metadataList,
			},
			BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
				CnsBackingObjectDetails: cnstypes.CnsBackingObjectDetails{},
				BackingDiskId:           pv.Spec.CSI.VolumeHandle,
			},
		}
		klog.V(4).Infof("CSPFullSync: volume %v is not in CNS cache", pv.Spec.CSI.VolumeHandle)
		createSpecArray = append(createSpecArray, createSpec)
	}
	return createSpecArray
}

// constructCnsUpdateSpec construct CnsVolumeMetadataUpdateSpec for given list of PVs
func constructCnsUpdateSpec(pvList []*v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeMetadataUpdateSpec {
	var updateSpecArray []cnstypes.CnsVolumeMetadataUpdateSpec
	for _, pv := range pvList {
		// Create new metadata spec
		metadataList := buildMetadataList(pv, pvToPVCMap, pvcToPodMap)
		// volume exist in K8S and CNS cache, but metadata is different, need to update this volume
		updateSpec := cnstypes.CnsVolumeMetadataUpdateSpec{
			VolumeId: cnstypes.CnsVolumeId{
				Id: pv.Spec.CSI.VolumeHandle,
			},
			Metadata: cnstypes.CnsVolumeMetadata{
				ContainerCluster: cnsvsphere.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
				EntityMetadata:   metadataList,
			},
		}

		updateSpecArray = append(updateSpecArray, updateSpec)
		klog.V(4).Infof("CSPFullSync: update metadata for volume %v", pv.Spec.CSI.VolumeHandle)
	}
	return updateSpecArray
}

// buildPVCMapPodMap build two maps to help
//  1. find PVC for given PV
//  2. find POD mounted to given PVC
// pvToPVCMap maps PV name to corresponding PVC, key is pv name
// pvcToPodMap maps PVC to the POD attached to the PVC, key is "pvc.Namespace/pvc.Name"
func buildPVCMapPodMap(k8sclient clientset.Interface, pvList []*v1.PersistentVolume) (pvcMap, podMap, error) {
	pvToPVCMap := make(pvcMap)
	pvcToPodMap := make(podMap)
	for _, pv := range pvList {
		if pv.Spec.ClaimRef != nil {
			pvc, err := k8sclient.CoreV1().PersistentVolumeClaims(pv.Spec.ClaimRef.Namespace).Get(pv.Spec.ClaimRef.Name, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("CSPFullSync: Failed to get pvc for namespace %v and name %v", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
				return pvToPVCMap, pvcToPodMap, err
			}
			pvToPVCMap[pv.Name] = pvc
			klog.V(4).Infof("CSPFullSync: pvc %v is backed by pv %v", pvc.Name, pv.Name)
			pods, err := k8sclient.CoreV1().Pods(pvc.Namespace).List(metav1.ListOptions{})
			if err != nil {
				klog.Warningf("CSPFullSync: Failed to get pods for namespace %v", pvc.Namespace)
					return pvToPVCMap, pvcToPodMap, err
			}
			for _, pod := range pods.Items {
				if pod.Spec.Volumes != nil {
					for _, volume := range pod.Spec.Volumes {
						pvClaim := volume.VolumeSource.PersistentVolumeClaim
						if pvClaim != nil && pvClaim.ClaimName == pvc.Name {
							key := pod.Namespace + "/" + pvClaim.ClaimName
							pvcToPodMap[key] = &pod
							klog.V(4).Infof("CSPFullSync: pvc %v is mounted by pod %v", key, pod.Name)
							break
								}
							}
						}
			}

		}
	}
	return pvToPVCMap, pvcToPodMap, nil
}

