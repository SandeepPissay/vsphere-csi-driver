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
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	api "k8s.io/kubernetes/pkg/apis/core"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	volumes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	"sync"
)

const (
	// Constants for specifying operation that needs to be performed on CNS volume

	// Create the volume on CNS
	createVolumeOperation = "createVolume"
	// Update the volume entries on CNS
	updateVolumeOperation = "updateVolume"
	// Delete the PVC entry and Pod entry (if it exists) on CNS
	updateVolumeWithDeleteClaimOperation = "updateVolumeWithDeleteClaim"
	// Delete the Pod entry on CNS
	updateVolumeWithDeletePodOperation = "updateVolumeWithDeletePod"
)

type pvcMap = map[string]*v1.PersistentVolumeClaim
type podMap = map[string]*v1.Pod

// Create a mapping of CNS volume to Pod name
// as this mapping does not exist in K8s
// in case a Pod entry needs to be deleted from CNS cache
var cnsVolumeToPodMap map[string]string

// Create a mapping of CNS volume to Pvc name
var cnsVolumeToPvcMap map[string]string

// Create a mapping of CNS volume to entity Namespace name
// Here entity can be either PVC or Pod - both will
// belong to the same namespace
var cnsVolumeToEntityNamespaceMap map[string]string

// triggerFullSync triggers full sync
func triggerFullSync(k8sclient clientset.Interface, metadataSyncer *metadataSyncInformer) {
	klog.V(2).Infof("CSPFullSync: start")
	// Get all PVs from kubernetes
	allPVs, err := k8sclient.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
	if err != nil {
		klog.Warningf("CSPFullSync: Failed to get PVs from kubernetes. Err: %v", err)
		return
	}
	// Get PVs in State "Bound", "Available" or "Released"
	k8sPVs := getPVsInBoundAvailableOrReleased(allPVs)

	// pvToPVCMap maps pv name to corresponding PVC
	// pvcToPodMap maps pvc to the mounted Pod
	pvToPVCMap, pvcToPodMap := buildPVCMapPodMap(k8sclient, k8sPVs)
	klog.V(4).Infof("CSPFullSync: pvToPVCMap %v", pvToPVCMap)
	klog.V(4).Infof("CSPFullSync: pvcToPodMap %v", pvcToPodMap)

	//Call CNS QueryAll to get container volumes by cluster ID
	queryFilter := cnstypes.CnsQueryFilter{
		ContainerClusterIds: []string{
			metadataSyncer.cfg.Global.ClusterID,
		},
	}
	querySelection := cnstypes.CnsQuerySelection{}
	queryAllResult, err := volumes.GetManager(metadataSyncer.vcenter).QueryAllVolume(queryFilter, querySelection)
	if err != nil {
		klog.Warningf("CSPFullSync: failed to queryAllVolume with err %v", err)
		return
	}
	cnsVolumeArray := queryAllResult.Volumes

	// Initialize CNS volume maps
	cnsVolumeToPodMap = make(map[string]string)
	cnsVolumeToPvcMap = make(map[string]string)
	cnsVolumeToEntityNamespaceMap = make(map[string]string)

	// Map K8s PV's to the operation that needs to be performed on them
	k8sPVsMap := buildVolumeMap(k8sPVs, cnsVolumeArray, pvToPVCMap, pvcToPodMap, metadataSyncer)
	klog.V(4).Infof("CSPFullSync: k8sPVMap %v", k8sPVsMap)

	// Identify volumes to be created, updated and deleted
	volToBeCreated, volToBeUpdated, volWithPvcEntryToBeDeleted, volWithPodEntryToBeDeleted := identifyVolumesToBeCreatedUpdated(k8sPVs, k8sPVsMap)
	volToBeDeleted := identifyVolumesToBeDeleted(cnsVolumeArray, k8sPVsMap)

	// Construct the cns spec for create and update operations
	createSpecArray := constructCnsCreateSpec(volToBeCreated, pvToPVCMap, pvcToPodMap, metadataSyncer)
	updateSpecArray := constructCnsUpdateSpec(volToBeUpdated, pvToPVCMap, pvcToPodMap, metadataSyncer)
	updateSpecArray = append(updateSpecArray, constructCnsUpdateSpecWithPVCToBeDeleted(volWithPvcEntryToBeDeleted, metadataSyncer)...)
	updateSpecArray = append(updateSpecArray, constructCnsUpdateSpecWithPodToBeDeleted(volWithPodEntryToBeDeleted, metadataSyncer)...)

	wg := sync.WaitGroup{}
	wg.Add(3)
	// Perform operations
	go fullSyncCreateVolumes(createSpecArray, metadataSyncer, &wg)
	go fullSyncDeleteVolumes(volToBeDeleted, metadataSyncer, &wg)
	go fullSyncUpdateVolumes(updateSpecArray, metadataSyncer, &wg)
	wg.Wait()
	klog.V(4).Infof("CSPFullSync: cnsDeletionMap at end of cycle: %v", cnsDeletionMap)
	klog.V(2).Infof("CSPFullSync: end")
}

// getPVsInBoundAvailableOrReleased return PVs in Bound, Available or Released state
func getPVsInBoundAvailableOrReleased(pvList *v1.PersistentVolumeList) []*v1.PersistentVolume {
	var pvsInDesiredState []*v1.PersistentVolume
	for index, pv := range pvList.Items {
		if pv.Spec.CSI.Driver == service.Name {
			klog.V(4).Infof("CSPFullSync: pv %v is in state %v", pv.Spec.CSI.VolumeHandle, pv.Status.Phase)
			if pv.Status.Phase == v1.VolumeBound || pv.Status.Phase == v1.VolumeAvailable || pv.Status.Phase == v1.VolumeReleased {
				pvsInDesiredState = append(pvsInDesiredState, &pvList.Items[index])
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
// If the volume is successfully deleted, it is removed from cnsDeletionMap
func fullSyncDeleteVolumes(volumeIDDeleteArray []cnstypes.CnsVolumeId, metadataSyncer *metadataSyncInformer, wg *sync.WaitGroup) {
	deleteDisk := false
	for _, volID := range volumeIDDeleteArray {
		klog.V(4).Infof("CSPFullSync: Calling DeleteVolume for volume %v with delete disk %v", volID, deleteDisk)
		err := volumes.GetManager(metadataSyncer.vcenter).DeleteVolume(volID.Id, deleteDisk)
		if err != nil {
			klog.Warningf("CSPFullSync: Failed to delete volume %s with error %+v", volID, err)
			continue
		}
		delete(cnsDeletionMap, volID.Id)
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

// buildCnsUpdateMetadataList build metadata list for given PV
// metadata list may include PV metadata, PVC metadata and POD metadata
func buildCnsUpdateMetadataList(pv *v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap) []cnstypes.BaseCnsEntityMetadata {
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
// A volume mapped to an empty string implies either no operation has to be performed or that the volume will be
// deleted
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
					{
						Id: pv.Spec.CSI.VolumeHandle,
					},
				},
			}

			queryResult, err := volumes.GetManager(metadataSyncer.vcenter).QueryVolume(queryFilter)
			if err == nil && queryResult != nil && len(queryResult.Volumes) > 0 {
				if &queryResult.Volumes[0].Metadata != nil {
					cnsMetadata := queryResult.Volumes[0].Metadata.EntityMetadata
					metadataList := buildCnsUpdateMetadataList(pv, pvToPVCMap, pvcToPodMap)
					k8sPVMap[pv.Spec.CSI.VolumeHandle] = getCnsUpdateOperationType(metadataList, cnsMetadata, pv.Name)
				} else {
					// metadata does not exist in CNS cache even the volume has an entry in CNS cache
					klog.Warningf("CSPFullSync: No metadata found for volume %v", pv.Spec.CSI.VolumeHandle)
					k8sPVMap[pv.Spec.CSI.VolumeHandle] = updateVolumeOperation
				}
			}
		} else {
			// PV exist in K8S but not in CNS cache, need to create
			k8sPVMap[pv.Spec.CSI.VolumeHandle] = createVolumeOperation
		}
	}

	return k8sPVMap
}

// identifyVolumesToBeCreatedUpdated return list of PV need to be created and updated
// volumes to be updated can be of three types -
// 	1. volumes whose existing metadata needs to be updated/created
//  2. volumes whose existing PVC and Pod metadata needs to be deleted
// 	3. volumes whose existing Pod metadata needs to be deleted
func identifyVolumesToBeCreatedUpdated(pvList []*v1.PersistentVolume, k8sPVMap map[string]string) ([]*v1.PersistentVolume, []*v1.PersistentVolume, []*v1.PersistentVolume, []*v1.PersistentVolume) {
	pvToBeCreated := []*v1.PersistentVolume{}
	pvToBeUpdated := []*v1.PersistentVolume{}
	pvcToBeDeleted := []*v1.PersistentVolume{}
	podToBeDeleted := []*v1.PersistentVolume{}
	for _, pv := range pvList {
		switch k8sPVMap[pv.Spec.CSI.VolumeHandle] {
		case createVolumeOperation:
			klog.V(4).Infof("CSPFullSync: Volume with id %s added to volume create list", pv.Spec.CSI.VolumeHandle)
			pvToBeCreated = append(pvToBeCreated, pv)
		case updateVolumeOperation:
			klog.V(4).Infof("CSPFullSync: Volume with id %s added to volume update list", pv.Spec.CSI.VolumeHandle)
			pvToBeUpdated = append(pvToBeUpdated, pv)
		case updateVolumeWithDeleteClaimOperation:
			klog.V(4).Infof("CSPFullSync: Volume with id %s and claim %s added to volume claim delete list", pv.Spec.CSI.VolumeHandle, cnsVolumeToPvcMap[pv.Name])
			pvcToBeDeleted = append(pvcToBeDeleted, pv)
		case updateVolumeWithDeletePodOperation:
			klog.V(4).Infof("CSPFullSync: Volume with id %s and pod name %s added to volume pod delete list", pv.Spec.CSI.VolumeHandle, cnsVolumeToPodMap[pv.Name])
			podToBeDeleted = append(podToBeDeleted, pv)
		}
	}
	return pvToBeCreated, pvToBeUpdated, pvcToBeDeleted, podToBeDeleted
}

// identifyVolumesToBeDeleted return list of volumeId's that need to be deleted
// A volumeId is added to this list only if it was present in cnsDeletionMap across two
// cycles of full sync
func identifyVolumesToBeDeleted(cnsVolumeList []cnstypes.CnsVolume, k8sPVMap map[string]string) []cnstypes.CnsVolumeId {
	var volToBeDeleted []cnstypes.CnsVolumeId
	for _, vol := range cnsVolumeList {
		_, existsInK8s := k8sPVMap[vol.VolumeId.Id]
		_, existsInCnsDeletionMap := cnsDeletionMap[vol.VolumeId.Id]
		if !existsInK8s {
			if existsInCnsDeletionMap {
				// Volume does not exist in K8s across two fullsync cycles - add to delete list
				klog.V(4).Infof("CSPFullSync: Volume with id %s added to delete list as it was present in cnsDeletionMap across two fullsync cycles", vol.VolumeId.Id)
				volToBeDeleted = append(volToBeDeleted, vol.VolumeId)
			} else {
				// Add to cnsDeletionMap
				klog.V(4).Infof("Volume with id %s added to cnsDeletionMap", vol.VolumeId.Id)
				cnsDeletionMap[vol.VolumeId.Id] = true
			}
		} else {
			if existsInCnsDeletionMap {
				// Exists in K8s and CNS, can remove from cnsDeletionMap
				klog.V(4).Infof("Volume with id %s removed from cnsDeletionMap", vol.VolumeId.Id)
				delete(cnsDeletionMap, vol.VolumeId.Id)
			}
		}
	}
	return volToBeDeleted
}

// constructCnsCreateSpec construct CnsVolumeCreateSpec for given list of PVs
func constructCnsCreateSpec(pvList []*v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeCreateSpec {
	var createSpecArray []cnstypes.CnsVolumeCreateSpec
	for _, pv := range pvList {
		// Create new metadata spec
		metadataList := buildCnsUpdateMetadataList(pv, pvToPVCMap, pvcToPodMap)
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
func constructCnsUpdateSpec(pvUpdateList []*v1.PersistentVolume, pvToPVCMap pvcMap, pvcToPodMap podMap, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeMetadataUpdateSpec {
	var updateSpecArray []cnstypes.CnsVolumeMetadataUpdateSpec
	for _, pv := range pvUpdateList {
		// Create new metadata spec with delete flag false
		metadataList := buildCnsUpdateMetadataList(pv, pvToPVCMap, pvcToPodMap)
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
		klog.V(4).Infof("CSPFullSync: constructCnsUpdateSpec to update metadata for volume %s with delete flag false", pv.Spec.CSI.VolumeHandle)
	}

	return updateSpecArray
}

// constructCnsUpdateSpecWithPVCToBeDeleted constructs CnsVolumeMetadataUpdateSpec for given list of PVs
// List of PVs have PVC and/or Pod entries in CNS that need to be deleted
func constructCnsUpdateSpecWithPVCToBeDeleted(pvUpdateList []*v1.PersistentVolume, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeMetadataUpdateSpec {
	var updateSpecArray []cnstypes.CnsVolumeMetadataUpdateSpec

	for _, pv := range pvUpdateList {
		updateSpec := buildCnsMetadataSpecMarkedForDelete(pv, updateVolumeWithDeleteClaimOperation)
		// volume exist in K8S and CNS cache, but PVC metadata does not exist in K8S
		// need to delete PVC entries for this volume
		updateSpec.Metadata.ContainerCluster = cnsvsphere.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User)
		updateSpecArray = append(updateSpecArray, updateSpec)
		klog.V(4).Infof("CSPFullSync: constructCnsUpdateSpecWithPVCToBeDeleted to update metadata for volume %s with delete flag true", pv.Spec.CSI.VolumeHandle)
	}
	return updateSpecArray
}

// constructCnsUpdateSpecWithPodToBeDeleted constructs CnsVolumeMetadataUpdateSpec for given list of PVs
// List of PVs have Pod entries in CNS that need to be deleted
func constructCnsUpdateSpecWithPodToBeDeleted(pvUpdateList []*v1.PersistentVolume, metadataSyncer *metadataSyncInformer) []cnstypes.CnsVolumeMetadataUpdateSpec {
	var updateSpecArray []cnstypes.CnsVolumeMetadataUpdateSpec

	for _, pv := range pvUpdateList {
		updateSpec := buildCnsMetadataSpecMarkedForDelete(pv, updateVolumeWithDeletePodOperation)
		// volume exist in K8S and CNS cache, but Pod metadata does not exist in K8S
		// need to delete Pod entries for this volume
		updateSpec.Metadata.ContainerCluster = cnsvsphere.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User)
		updateSpecArray = append(updateSpecArray, updateSpec)
		klog.V(4).Infof("CSPFullSync: constructCnsUpdateSpecWithPodToBeDeleted to update metadata for volume %s with delete flag true", pv.Spec.CSI.VolumeHandle)
	}

	return updateSpecArray
}

// buildPVCMapPodMap build two maps to help
//  1. find PVC for given PV
//  2. find POD mounted to given PVC
// pvToPVCMap maps PV name to corresponding PVC, key is pv name
// pvcToPodMap maps PVC to the POD attached to the PVC, key is "pvc.Namespace/pvc.Name"
func buildPVCMapPodMap(k8sclient clientset.Interface, pvList []*v1.PersistentVolume) (pvcMap, podMap) {
	pvToPVCMap := make(pvcMap)
	pvcToPodMap := make(podMap)
	for _, pv := range pvList {
		if pv.Spec.ClaimRef != nil && pv.Status.Phase == v1.VolumeBound {
			pvc, err := k8sclient.CoreV1().PersistentVolumeClaims(pv.Spec.ClaimRef.Namespace).Get(pv.Spec.ClaimRef.Name, metav1.GetOptions{})
			if err != nil {
				klog.Warningf("CSPFullSync: Failed to get pvc for namespace %v and name %v. err=%v", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name, err)
				continue
			}
			pvToPVCMap[pv.Name] = pvc
			klog.V(4).Infof("CSPFullSync: pvc %v is backed by pv %v", pvc.Name, pv.Name)
			pods, err := k8sclient.CoreV1().Pods(pvc.Namespace).List(metav1.ListOptions{
				FieldSelector: fields.AndSelectors(fields.SelectorFromSet(fields.Set{"status.phase": string(api.PodRunning)})).String(),
			})
			if err != nil {
				klog.Warningf("CSPFullSync: Failed to get pods for namespace %v. err=%v", pvc.Namespace, err)
				continue
			}
			for index, pod := range pods.Items {
				if pod.Spec.Volumes != nil {
					for _, volume := range pod.Spec.Volumes {
						pvClaim := volume.VolumeSource.PersistentVolumeClaim
						if pvClaim != nil && pvClaim.ClaimName == pvc.Name {
							key := pod.Namespace + "/" + pvClaim.ClaimName
							pvcToPodMap[key] = &pods.Items[index]
							klog.V(4).Infof("CSPFullSync: pvc %v is mounted by pod %v", key, pod.Name)
							break
						}
					}
				}
			}

		}
	}
	return pvToPVCMap, pvcToPodMap
}

// getCnsUpdateOperationType compares the input metadata list from K8S and metadata list from CNS
// Returns the update operation type that needs to be performed on CNS
// Empty string returned implies either no operation needs to be performed or
// volume needs to be deleted from CNS
func getCnsUpdateOperationType(pvMetadataList []cnstypes.BaseCnsEntityMetadata, cnsMetadataList []cnstypes.BaseCnsEntityMetadata, pvName string) string {
	// K8s resource metadata contains more entries than CNS - need to update
	if len(pvMetadataList) > len(cnsMetadataList) {
		return updateVolumeOperation
	}

	// K8s resource metadata contains lesser entries than CNS - need to delete
	// some entries from CNS
	if len(pvMetadataList) < len(cnsMetadataList) {
		// Construct CNS volume mappings
		for _, cnsMetadata := range cnsMetadataList {
			// Construct CNS volume to Pod name mapping
			if cnsMetadata.(*cnstypes.CnsKubernetesEntityMetadata).EntityType == string(cnstypes.CnsKubernetesEntityTypePOD) {
				cnsVolumeToPodMap[pvName] = cnsMetadata.GetCnsEntityMetadata().EntityName
				cnsVolumeToEntityNamespaceMap[pvName] = cnsMetadata.(*cnstypes.CnsKubernetesEntityMetadata).Namespace
			}
			// Construct CNS volume to Pvc name mapping
			if cnsMetadata.(*cnstypes.CnsKubernetesEntityMetadata).EntityType == string(cnstypes.CnsKubernetesEntityTypePVC) {
				cnsVolumeToPvcMap[pvName] = cnsMetadata.GetCnsEntityMetadata().EntityName
				cnsVolumeToEntityNamespaceMap[pvName] = cnsMetadata.(*cnstypes.CnsKubernetesEntityMetadata).Namespace
			}
		}
		// PVC and Pod entries need to be deleted from CNS
		// as K8s metadata only contains PV entry
		if len(pvMetadataList) == 1 {
			return updateVolumeWithDeleteClaimOperation
		}
		// Pod entry needs to be deleted from CNS
		// as K8s metadata only PV and PVC entry
		if len(pvMetadataList) == 2 {
			return updateVolumeWithDeletePodOperation
		}
	}

	// Same number of entries for volume in K8s and CNS
	// Need to check if entries match
	cnsMetadataMap := make(map[string]*cnstypes.CnsKubernetesEntityMetadata)
	for _, cnsMetadata := range cnsMetadataList {
		cnsKubernetesMetadata := cnsMetadata.(*cnstypes.CnsKubernetesEntityMetadata)
		cnsMetadataMap[cnsKubernetesMetadata.EntityType] = cnsKubernetesMetadata
	}
	for _, k8sMetadata := range pvMetadataList {
		k8sKubernetesMetadata := k8sMetadata.(*cnstypes.CnsKubernetesEntityMetadata)
		if _, ok := cnsMetadataMap[k8sKubernetesMetadata.EntityType]; ok && !cnsvsphere.CompareKubernetesMetadata(k8sKubernetesMetadata, cnsMetadataMap[k8sKubernetesMetadata.EntityType]) {
			return updateVolumeOperation
		}
	}
	return ""
}

// buildCnsMetadataSpecMarkedForDelete builds metadata list for a volume
// where PVC and/or Pod entries need to be deleted from CNS
// and returns the update spec to be passed to CNS
func buildCnsMetadataSpecMarkedForDelete(pv *v1.PersistentVolume, operationType string) cnstypes.CnsVolumeMetadataUpdateSpec {
	// Create new metadata spec with delete flag true
	var metadataList []cnstypes.BaseCnsEntityMetadata
	if _, ok := cnsVolumeToPvcMap[pv.Name]; ok && operationType == updateVolumeWithDeleteClaimOperation {
		pvcMetadata := cnsvsphere.GetCnsKubernetesEntityMetaData(cnsVolumeToPvcMap[pv.Name], nil, true, string(cnstypes.CnsKubernetesEntityTypePVC), cnsVolumeToEntityNamespaceMap[pv.Name])
		metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvcMetadata))
	}
	if _, ok := cnsVolumeToPodMap[pv.Name]; ok {
		podMetadata := cnsvsphere.GetCnsKubernetesEntityMetaData(cnsVolumeToPodMap[pv.Name], nil, true, string(cnstypes.CnsKubernetesEntityTypePOD), cnsVolumeToEntityNamespaceMap[pv.Name])
		metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(podMetadata))
	}

	updateSpec := cnstypes.CnsVolumeMetadataUpdateSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: pv.Spec.CSI.VolumeHandle,
		},
		Metadata: cnstypes.CnsVolumeMetadata{
			EntityMetadata: metadataList,
		},
	}
	return updateSpec
}
