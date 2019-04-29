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
	cnstypes "gitlab.eng.vmware.com/hatchway/common-csp/cns/types"
	volumes "gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume"
	volumestypes "gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume/types"
	cnsvsphere "gitlab.eng.vmware.com/hatchway/common-csp/pkg/vsphere"
	"k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	"os"
	"reflect"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	vTypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

const (
	csidriver = "vsphere.csi.vmware.com"
)

type metadataSyncInformer struct {
	cfg                  *cnsconfig.Config
	cnsconfig            *cnsvsphere.VirtualCenterConfig
	k8sInformerManager   *k8s.InformerManager
	virtualcentermanager cnsvsphere.VirtualCenterManager
	PVLister             corelisters.PersistentVolumeLister
}

// Returns uninitialized metadataSyncInformer
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

	metadataSyncer.cnsconfig, err = block.GetVirtualCenterConfig(metadataSyncer.cfg)
	if err != nil {
		klog.Errorf("Failed to get VirtualCenterConfig. err=%v", err)
		return err
	}

	// Initialize the virtual center manager
	metadataSyncer.virtualcentermanager = cnsvsphere.GetVirtualCenterManager()

	// Register virtual center manager
	vcenter, err := metadataSyncer.virtualcentermanager.RegisterVirtualCenter(metadataSyncer.cnsconfig)
	if err != nil {
		klog.Errorf("Failed to register VirtualCenter . err=%v", err)
		return err
	}
	klog.Infof("vcenter returned: %s", spew.Sdump(vcenter))
	// Connect to VC
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err = vcenter.Connect(ctx)
	if err != nil {
		klog.Errorf("Failed to connect to VirtualCenter host: %q. err=%v", metadataSyncer.cnsconfig.Host, err)
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
	metadataSyncer.k8sInformerManager.AddPVCListener(nil, PVCUpdated, PVCDeleted)
	metadataSyncer.k8sInformerManager.AddPVListener(
		nil, // Add
		func(oldObj interface{}, newObj interface{}) { // Update
			PVUpdated(oldObj, newObj, metadataSyncer)
		},
		func(obj interface{}) { // Delete
			PVDeleted(obj, metadataSyncer)
		})
	metadataSyncer.k8sInformerManager.AddPodListener(nil, PodUpdated, PodDeleted)

	metadataSyncer.PVLister = metadataSyncer.k8sInformerManager.GetPVLister()
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

func PVCUpdated(oldObj, newObj interface{}) {
	fmt.Printf("Temporary implementation of PVC Update\n")
}

func PVCDeleted(obj interface{}) {
	fmt.Printf("Temporary implementation of PVC Delete\n")
}

func PVUpdated(oldObj, newObj interface{}, metadataSyncer *metadataSyncInformer) {
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
	klog.V(5).Infof("PVUpdate: Updating PV from %+v to %+v", oldPv, newPv)

	// Check if vsphere volume
	if newPv.Spec.CSI.Driver != csidriver {
		klog.V(5).Infof("PVUpdate: PV is not a vsphere volume: %+v", newPv)
		return
	}
	if newPv.Status.Phase == v1.VolumePending || newPv.Status.Phase == v1.VolumeFailed {
		klog.V(3).Infof("PVUpdate: PV %s metadata is not updated since updated PV is in phase %s", newPv.Name, newPv.Status.Phase)
		return
	}
	vCenterIP, err := block.GetVcenterIPs(metadataSyncer.cfg)
	if err != nil {
		klog.Errorf("Failed to get vCenter from config. Err: %+v", err)
		return
	}
	vc, err := metadataSyncer.virtualcentermanager.GetVirtualCenter(vCenterIP[0])
	if err != nil {
		klog.Errorf("Cannot get virtual center object for server %s with error %+v", vc.Config.Host, err)
		return
	}

	newLabels := newPv.GetLabels()
	if oldPv.Status.Phase == v1.VolumeAvailable && newPv.Status.Phase == v1.VolumeAvailable && reflect.DeepEqual(newLabels, oldPv.GetLabels()) {
		klog.V(5).Infof("PVUpdate: PV labels have not changed")
		return
	}

	var metadataList []volumestypes.EntityMetaData

	if (oldPv.Status.Phase == v1.VolumeAvailable && newPv.Status.Phase == v1.VolumeAvailable) || newPv.Spec.StorageClassName != "" {
		pvMetadata := block.GetEntityMetaData(newPv.Name, newPv.Namespace,
			string(cnstypes.CnsKubernetesEntityTypePV),
			newPv.Labels, false)
		metadataList = append(metadataList, pvMetadata)

		updateSpec := &volumestypes.UpdateSpec{
			VolumeID: &volumestypes.VolumeID{
				ID: newPv.Spec.CSI.VolumeHandle,
			},
			ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[vc.Config.Host].User),
			MetaData:         metadataList,
		}

		klog.V(4).Infof("PVUpdated: Calling UpdateVolumeMetadata for volume %s with updateSpec: %+v", updateSpec.VolumeID.ID, spew.Sdump(updateSpec))
		volumes.GetManager(vc).UpdateVolumeMetadata(updateSpec)
		return
	} else {
		pvMetadata := block.GetEntityMetaData(oldPv.Name, newPv.Namespace,
			string(cnstypes.CnsKubernetesEntityTypePV),
			newPv.Labels, false)
		metadataList = append(metadataList, pvMetadata)

		createSpec := &volumestypes.CreateSpec{
			Name: oldPv.Name,
			BackingInfo: &volumestypes.BlockBackingInfo{
				BackingDiskID: newPv.Spec.CSI.VolumeHandle,
			},
			ContainerCluster: block.GetContainerCluster(metadataSyncer.cfg.Global.ClusterID, metadataSyncer.cfg.VirtualCenter[vc.Config.Host].User),
			MetaData:         metadataList,
		}

		klog.V(4).Infof("PVUpdate: vSphere Cloud Provider creating volume %s with create spec %+v", oldPv.Name, spew.Sdump(createSpec))
		_, err = volumes.GetManager(vc).CreateVolume(createSpec)

		if err != nil {
			klog.Errorf("Failed to create disk %s with error %+v", oldPv.Name, err)
		}
	}
}

func PVDeleted(obj interface{}, metadataSyncer *metadataSyncInformer) {
	pv, ok := obj.(*v1.PersistentVolume)
	if pv == nil || !ok {
		klog.Warningf("PVDeleted: unrecognized object %+v", obj)
		return
	}
	klog.V(5).Infof("PVDelete: Deleting PV: %+v", pv)

	// Check if vsphere volume
	if pv.Spec.CSI.Driver != csidriver {
		klog.V(5).Infof("PVDelete: Not a vsphere volume: %+v", pv)
		return
	}
	vCenterIP, err := block.GetVcenterIPs(metadataSyncer.cfg)
	if err != nil {
		klog.Errorf("Failed to get vCenter from config. Err: %+v", err)
		return
	}
	vc, err := metadataSyncer.virtualcentermanager.GetVirtualCenter(vCenterIP[0])
	if err != nil {
		klog.Errorf("Cannot get virtual center object for server %s with error %+v", vc.Config.Host, err)
		return
	}
	deleteSpec := &volumestypes.DeleteSpec{
		VolumeID: &volumestypes.VolumeID{
			ID: pv.Spec.CSI.VolumeHandle,
		},
	}
	if pv.Spec.ClaimRef == nil || (pv.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete) {
		deleteSpec.DeleteDisk = false
		err = volumes.GetManager(vc).DeleteVolume(deleteSpec)
		if err != nil {
			klog.Errorf("PVDelete: Failed to delete disk %s with error %+v", deleteSpec.VolumeID.ID, err)
			return
		}
	} else {
		// We set delete disk=true for the case where PV status is failed and kubernetes has deleted the volume after timing out
		// In this case, the syncer will remove the volume from VC
		deleteSpec.DeleteDisk = true
		err = volumes.GetManager(vc).DeleteVolume(deleteSpec)
		//e := errors.New("ServerFaultCode: NotFound\n")
		if err != nil {
			klog.Warningf("PVDelete: Failed to delete disk %s with error %+v", deleteSpec.VolumeID.ID, err)
			return
		}

	}

}

func PodUpdated(oldObj, newObj interface{}) {
	fmt.Printf("Temporary implementation of Pod Update\n")
}

func PodDeleted(obj interface{}) {
	fmt.Printf("Temporary implementation of Pod Delete\n")
}

