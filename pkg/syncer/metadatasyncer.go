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
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	csictx "github.com/rexray/gocsi/context"
	"k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	"os"
	"reflect"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	volumes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	vTypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

type metadataSyncInformer struct {
	cfg                  *cnsconfig.Config
	vcconfig             *cnsvsphere.VirtualCenterConfig
	k8sInformerManager   *k8s.InformerManager
	virtualcentermanager cnsvsphere.VirtualCenterManager
	vcenter              *cnsvsphere.VirtualCenter
	pvLister             corelisters.PersistentVolumeLister
}

// New Returns uninitialized metadataSyncInformer
func New() *metadataSyncInformer {
	return &metadataSyncInformer{}
}

// Initializes the Metadata Sync Informer
func (metadataSyncer *metadataSyncInformer) Init() error {
	var err error

	// Create and read config from vsphere.conf
	metadataSyncer.cfg, err = createAndReadConfig()
	if err != nil {
		klog.Errorf("Failed to parse config. Err: %v", err)
		return err
	}

	metadataSyncer.vcconfig, err = block.GetVirtualCenterConfig(metadataSyncer.cfg)
	if err != nil {
		klog.Errorf("Failed to get VirtualCenterConfig. err=%v", err)
		return err
	}

	// Initialize the virtual center manager
	metadataSyncer.virtualcentermanager = cnsvsphere.GetVirtualCenterManager()

	// Register virtual center manager
	metadataSyncer.vcenter, err = metadataSyncer.virtualcentermanager.RegisterVirtualCenter(metadataSyncer.vcconfig)
	if err != nil {
		klog.Errorf("Failed to register VirtualCenter . err=%v", err)
		return err
	}

	// Connect to VC
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = metadataSyncer.vcenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to VirtualCenter host: %q. err=%v", metadataSyncer.vcconfig.Host, err)
		return err
	}
	// Create the kubernetes client from config
	k8sclient, err := k8s.NewClient(metadataSyncer.cfg.Global.ServiceAccount)
	if err != nil {
		klog.Errorf("Creating Kubernetes client failed. Err: %v", err)
		return err
	}
	// Set up kubernetes resource listeners for metadata syncer
	metadataSyncer.k8sInformerManager = k8s.NewInformer(k8sclient)
	metadataSyncer.k8sInformerManager.AddPVCListener(
		nil, // Add
		func(oldObj interface{}, newObj interface{}) { // Update
			pvcUpdated(oldObj, newObj, metadataSyncer)
		},
		func(obj interface{}) { // Delete
			pvcDeleted(obj, metadataSyncer)
		})
	metadataSyncer.k8sInformerManager.AddPVListener(
		nil, // Add
		func(oldObj interface{}, newObj interface{}) { // Update
			pvUpdated(oldObj, newObj, metadataSyncer)
		},
		func(obj interface{}) { // Delete
			pvDeleted(obj, metadataSyncer)
		})
	metadataSyncer.k8sInformerManager.AddPodListener(nil, podUpdated, podDeleted)
	metadataSyncer.pvLister = metadataSyncer.k8sInformerManager.GetPVLister()
	klog.V(2).Infof("Initialized metadata syncer")
	stopCh := metadataSyncer.k8sInformerManager.Listen()
	<-(stopCh)
	return nil
}

func createAndReadConfig() (*cnsconfig.Config, error) {
	var cfg *cnsconfig.Config
	var cfgPath = vTypes.DefaultCloudConfigPath

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfgPath = csictx.Getenv(ctx, vTypes.EnvCloudConfig)
	if cfgPath == "" {
		cfgPath = vTypes.DefaultCloudConfigPath
	}

	//Read in the vsphere.conf if it exists
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		// config from Env var only
		cfg = &cnsconfig.Config{}
		if err := cnsconfig.FromEnv(cfg); err != nil {
			klog.Errorf("error reading vsphere.conf\n")
			return cfg, err
		}
	} else {
		config, err := os.Open(cfgPath)
		if err != nil {
			klog.Errorf("Failed to open %s. Err: %v", cfgPath, err)
			return cfg, err
		}
		cfg, err = cnsconfig.ReadConfig(config)
		if err != nil {
			klog.Errorf("Failed to parse config. Err: %v", err)
			return cfg, err
		}
	}
	return cfg, nil
}

func pvcUpdated(oldObj, newObj interface{}, metadataSyncer *metadataSyncInformer) {
	// Get old and new pvc objects
	oldPvc, ok := oldObj.(*v1.PersistentVolumeClaim)
	if oldPvc == nil || !ok {
		return
	}
	newPvc, ok := newObj.(*v1.PersistentVolumeClaim)
	if newPvc == nil || !ok {
		return
	}

	if newPvc.Status.Phase != v1.ClaimBound {
		klog.V(3).Infof("PVCUpdated: New PVC not in Bound phase")
		return
	}

	// Get pv object attached to pvc
	pv, err := block.GetPersistentVolume(newPvc, metadataSyncer.pvLister)
	if err != nil {
		klog.Errorf("PVCUpdated: Error getting Persistent Volume for pvc %q with err: %v", newPvc.UID, err)
		return
	}

	// Verify if pv is vsphere volume
	if pv.Spec.CSI.Driver != service.Name {
		klog.V(3).Infof("PVCUpdated: Not a Vsphere Volume")
		return
	}

	// Verify is old and new labels are not equal
	if oldPvc.Status.Phase == v1.ClaimBound && reflect.DeepEqual(newPvc.Labels, oldPvc.Labels) {
		klog.V(3).Infof("PVCUpdated: Old PVC and New PVC labels equal")
		return
	}

	// Create updateSpec
	var metadataList []cnstypes.BaseCnsEntityMetadata
	pvcMetadata := block.GetCnsKubernetesEntityMetaData(newPvc.Name, newPvc.Labels, false, string(cnstypes.CnsKubernetesEntityTypePVC), newPvc.Namespace)
	metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvcMetadata))

	updateSpec := &cnstypes.CnsVolumeMetadataUpdateSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: pv.Spec.CSI.VolumeHandle,
		},
		Metadata: cnstypes.CnsVolumeMetadata{
			ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
			EntityMetadata:   metadataList,
		},
	}

	klog.V(4).Infof("PVCUpdated: Calling UpdateVolumeMetadata with updateSpec: %+v", spew.Sdump(updateSpec))
	if err := volumes.GetManager(metadataSyncer.vcenter).UpdateVolumeMetadata(updateSpec); err != nil {
		klog.Errorf("PVCUpdated: UpdateVolumeMetadata failed with err %v", err)
	}
}

func pvcDeleted(obj interface{}, metadataSyncer *metadataSyncInformer) {
	pvc, ok := obj.(*v1.PersistentVolumeClaim)
	if pvc == nil || !ok {
		klog.Warningf("PVCDeleted: unrecognized object %+v", obj)
		return
	}
	klog.V(4).Infof("PVCDeleted: %+v", pvc)

	// Get pv object attached to pvc
	pv, err := block.GetPersistentVolume(pvc, metadataSyncer.pvLister)
	if err != nil {
		klog.Errorf("PVCDeleted: Error getting Persistent Volume for pvc %q with err: %v", pvc.UID, err)
		return
	}

	// Verify if pv is a vsphere volume
	if pv.Spec.CSI.Driver != service.Name {
		klog.V(3).Infof("PVCDeleted: Not a Vsphere Volume")
		return
	}

	// Volume will be deleted by controller when reclaim policy is delete
	if pv.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimDelete {
		klog.V(3).Infof("PVCDeleted: Reclaim policy is delete")
		return
	}

	// If the PV reclaim policy is retain we need to delete PVC labels
	var metadataList []cnstypes.BaseCnsEntityMetadata
	pvcMetadata := block.GetCnsKubernetesEntityMetaData(pvc.Name, nil, true, string(cnstypes.CnsKubernetesEntityTypePVC), pvc.Namespace)
	metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvcMetadata))

	updateSpec := &cnstypes.CnsVolumeMetadataUpdateSpec{
		VolumeId: cnstypes.CnsVolumeId{
			Id: pv.Spec.CSI.VolumeHandle,
		},
		Metadata: cnstypes.CnsVolumeMetadata{
			ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
			EntityMetadata:   metadataList,
		},
	}

	klog.V(4).Infof("PVCDeleted: Calling UpdateVolumeMetadata for volume %s with updateSpec: %+v", updateSpec.VolumeId.Id, spew.Sdump(updateSpec))
	if err := volumes.GetManager(metadataSyncer.vcenter).UpdateVolumeMetadata(updateSpec); err != nil {
		klog.Errorf("PVCDeleted: UpdateVolumeMetadata failed with err %v", err)
	}
}

// pvUpdated updates volume metadata on VC when volume labels on K8S cluster have been updated
func pvUpdated(oldObj, newObj interface{}, metadataSyncer *metadataSyncInformer) {
	// Get old and new PV objects
	oldPv, ok := oldObj.(*v1.PersistentVolume)
	if oldPv == nil || !ok {
		klog.Warningf("PVUpdated: unrecognized old object %+v", oldObj)
		return
	}

	newPv, ok := newObj.(*v1.PersistentVolume)
	if newPv == nil || !ok {
		klog.Warningf("PVUpdated: unrecognized new object %+v", newObj)
		return
	}
	klog.V(4).Infof("PVUpdate: PV Updated from %+v to %+v", oldPv, newPv)

	// Check if vsphere volume
	if newPv.Spec.CSI.Driver != service.Name {
		klog.V(3).Infof("PVUpdate: PV is not a vsphere volume: %+v", newPv)
		return
	}
	// Return if new PV status is Pending or Failed
	if newPv.Status.Phase == v1.VolumePending || newPv.Status.Phase == v1.VolumeFailed {
		klog.V(3).Infof("PVUpdate: PV %s metadata is not updated since updated PV is in phase %s", newPv.Name, newPv.Status.Phase)
		return
	}
	// Return if labels are unchanged
	if oldPv.Status.Phase == v1.VolumeAvailable && reflect.DeepEqual(newPv.GetLabels(), oldPv.GetLabels()) {
		klog.V(3).Infof("PVUpdate: PV labels have not changed")
		return
	}

	var metadataList []cnstypes.BaseCnsEntityMetadata
	pvMetadata := block.GetCnsKubernetesEntityMetaData(newPv.Name, newPv.GetLabels(), false, string(cnstypes.CnsKubernetesEntityTypePV), newPv.Namespace)
	metadataList = append(metadataList, cnstypes.BaseCnsEntityMetadata(pvMetadata))

	if oldPv.Status.Phase == v1.VolumeAvailable || newPv.Spec.StorageClassName != "" {
		updateSpec := &cnstypes.CnsVolumeMetadataUpdateSpec{
			VolumeId: cnstypes.CnsVolumeId{
				Id: newPv.Spec.CSI.VolumeHandle,
			},
			Metadata: cnstypes.CnsVolumeMetadata{
				ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
				EntityMetadata:   metadataList,
			},
		}

		klog.V(4).Infof("PVUpdated: Calling UpdateVolumeMetadata for volume %s with updateSpec: %+v", updateSpec.VolumeId.Id, spew.Sdump(updateSpec))
		if err := volumes.GetManager(metadataSyncer.vcenter).UpdateVolumeMetadata(updateSpec); err != nil {
			klog.Errorf("PVUpdate: UpdateVolumeMetadata failed with err %v", err)
		}
	} else {
		createSpec := &cnstypes.CnsVolumeCreateSpec{
			Name:       oldPv.Name,
			VolumeType: block.BlockVolumeType,
			Metadata: cnstypes.CnsVolumeMetadata{
				ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcenter.Config.Host].User),
				EntityMetadata:   metadataList,
			},
			BackingObjectDetails: &cnstypes.CnsBlockBackingDetails{
				CnsBackingObjectDetails: cnstypes.CnsBackingObjectDetails{},
				BackingDiskId:           oldPv.Spec.CSI.VolumeHandle,
			},
		}
		klog.V(4).Infof("PVUpdate: vSphere provisioner creating volume %s with create spec %+v", oldPv.Name, spew.Sdump(createSpec))
		_, err := volumes.GetManager(metadataSyncer.vcenter).CreateVolume(createSpec)

		if err != nil {
			klog.Errorf("PVUpdate: Failed to create disk %s with error %+v", oldPv.Name, err)
		}
	}
}

// pvDeleted deletes volume metadata on VC when volume has been deleted on K8s cluster
func pvDeleted(obj interface{}, metadataSyncer *metadataSyncInformer) {
	pv, ok := obj.(*v1.PersistentVolume)
	if pv == nil || !ok {
		klog.Warningf("PVDeleted: unrecognized object %+v", obj)
		return
	}
	klog.V(4).Infof("PVDeleted: Deleting PV: %+v", pv)

	// Check if vsphere volume
	if pv.Spec.CSI.Driver != service.Name {
		klog.V(3).Infof("PVDeleted: Not a vsphere volume: %+v", pv)
		return
	}

	var deleteDisk bool
	if pv.Spec.ClaimRef == nil || (pv.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete) {
		deleteDisk = false
	} else {
		// We set delete disk=true for the case where PV status is failed after deletion of pvc
		// In this case, metadatasyncer will remove the volume
		deleteDisk = true
	}

	klog.V(4).Infof("PVDeleted: vSphere provisioner deleting volume %v with delete disk %v", pv, deleteDisk)
	if err := volumes.GetManager(metadataSyncer.vcenter).DeleteVolume(pv.Spec.CSI.VolumeHandle, deleteDisk); err != nil {
		klog.Errorf("PVDeleted: Failed to delete disk %s with error %+v", pv.Spec.CSI.VolumeHandle, err)
		return
	}
}

func podUpdated(oldObj, newObj interface{}) {
	fmt.Printf("Temporary implementation of Pod Update\n")
}

func podDeleted(obj interface{}) {
	fmt.Printf("Temporary implementation of Pod Delete\n")
}
