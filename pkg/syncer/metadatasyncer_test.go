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
	"crypto/tls"
	"fmt"
	"k8s.io/klog"
	"log"
	"testing"

	"github.com/vmware/govmomi/simulator"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cnssim "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/simulator"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"

	"k8s.io/apimachinery/pkg/api/resource"
	clientset "k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

const (
	testVolumeName    = "test-volume"
	testClusterName   = "test-cluster"
	gbInMb            = 1024
	testVolumeType    = "BLOCK"
	testPVCLabelName  = "test-PVC-label"
	testPVCLabelValue = "test-PVC-value"
	testPVLabelName   = "test-PV-label"
	testPVLabelValue  = "test-PV-value"
	PVC               = "PVC"
	PV                = "PV"
	testNamespace     = "default"
)

// configFromSim starts a vcsim instance and returns config for use against the vcsim instance.
// The vcsim instance is configured with an empty tls.Config.
func configFromSim() (*cnsconfig.Config, func()) {
	return configFromSimWithTLS(new(tls.Config), true)
}

// configFromSimWithTLS starts a vcsim instance and returns config for use against the vcsim instance.
// The vcsim instance is configured with a tls.Config. The returned client
// config can be configured to allow/decline insecure connections.
func configFromSimWithTLS(tlsConfig *tls.Config, insecureAllowed bool) (*cnsconfig.Config, func()) {
	cfg := &cnsconfig.Config{}
	model := simulator.VPX()
	defer model.Remove()

	err := model.Create()
	if err != nil {
		log.Fatal(err)
	}

	model.Service.TLS = tlsConfig
	s := model.Service.NewServer()

	// CNS Service simulator
	model.Service.RegisterSDK(cnssim.New())

	cfg.Global.InsecureFlag = insecureAllowed

	cfg.Global.VCenterIP = s.URL.Hostname()
	cfg.Global.VCenterPort = s.URL.Port()
	cfg.Global.User = s.URL.User.Username()
	cfg.Global.Password, _ = s.URL.User.Password()
	cfg.Global.Datacenters = "DC0"
	cfg.VirtualCenter = make(map[string]*cnsconfig.VirtualCenterConfig)
	cfg.VirtualCenter[s.URL.Hostname()] = &cnsconfig.VirtualCenterConfig{
		User:         cfg.Global.User,
		Password:     cfg.Global.Password,
		VCenterPort:  cfg.Global.VCenterPort,
		InsecureFlag: cfg.Global.InsecureFlag,
		Datacenters:  cfg.Global.Datacenters,
	}

	return cfg, func() {
		s.Close()
		model.Remove()
	}

}

func configFromEnvOrSim() (*cnsconfig.Config, func()) {
	cfg := &cnsconfig.Config{}
	if err := cnsconfig.FromEnv(cfg); err != nil {
		return configFromSim()
	}
	return cfg, func() {}
}

func K8sClientFromEnvOrSim(metadataSyncer *metadataSyncInformer) (clientset.Interface, error) {
	// Create the kubernetes client from config or env
	var k8sclient clientset.Interface
	var err error
	if k8senv := os.Getenv("KUBECONFIG"); k8senv != "" {
		if k8sclient, err = k8s.NewClient(metadataSyncer.cfg.Global.ServiceAccount); err != nil {
			return nil, err
		}
	} else {
		k8sclient = testclient.NewSimpleClientset()
	}
	return k8sclient, nil
}

/*
	This test verifies the following workflows:
		1. pv update/delete for dynamically created pv
		2. pv update/delete for statically created pv
		3. pvc update/delete

	Test Steps:
		1. Setup configuration
			a. VC config is either simulated or derived from env
			b. k8sclient is simulated or derived from env - based on env varible USE_K8S_CLIENT
		2. Dynamically create a test volume on vc
		3. Verify pv update workflow
		4. Delete test volume with delete disk=false
		5. Statically create test volume on k8s with volumeHandle = recently deleted volume
		5. Verify pv update workflow
		6. Create pvc on k8s to bound to recently created pv
		7. Verify pvc update workflow
		8. Verify pvc delete workflow
		9. Verify pv delete workflow
*/

func TestMetadataSyncInformer(t *testing.T) {
	config, cleanup := configFromEnvOrSim()
	defer cleanup()

	// CNS based CSI requires a valid cluster name
	config.Global.ClusterID = testClusterName

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Init VC configuration
	cnsVCenterConfig, err := cnsvsphere.GetVirtualCenterConfig(config)
	if err != nil {
		t.Errorf("Failed to get virtualCenter. err=%v", err)
		t.Fatal(err)
	}

	cspVirtualCenterManager := cnsvsphere.GetVirtualCenterManager()

	cspVirtualCenter, err := cspVirtualCenterManager.RegisterVirtualCenter(cnsVCenterConfig)
	if err != nil {
		t.Fatal(err)
	}

	err = cspVirtualCenter.Connect(ctx)
	defer cspVirtualCenter.Disconnect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cspVolumeManager := volume.GetManager(cspVirtualCenter)

	// Initialize metadata syncer object
	var metadataSyncer *metadataSyncInformer
	metadataSyncer = &metadataSyncInformer{
		cfg:                  config,
		vcconfig:             cnsVCenterConfig,
		virtualcentermanager: cspVirtualCenterManager,
		vcenter:              cspVirtualCenter,
	}

	// Create the kubernetes client from config or env
	var k8sclient clientset.Interface
	if k8sclient, err = K8sClientFromEnvOrSim(metadataSyncer); err != nil {
		klog.Errorf("Creating Kubernetes client failed. Err: %v", err)
		return
	}
	metadataSyncer.k8sInformerManager = k8s.NewInformer(k8sclient)
	metadataSyncer.pvLister = metadataSyncer.k8sInformerManager.GetPVLister()
	metadataSyncer.k8sInformerManager.Listen()

	// Create spec for new volume
	var sharedDatastore string
	if v := os.Getenv("VSPHERE_DATASTORE"); v != "" {
		sharedDatastore = v
	} else {
		sharedDatastore = simulator.Map.Any("Datastore").(*simulator.Datastore).Info.GetDatastoreInfo().Name
	}

	dc, err := metadataSyncer.vcenter.GetDatacenters(ctx)
	if err != nil || len(dc) == 0 {
		t.Errorf("Failed to get datacenter for the path: %s. Error: %v", metadataSyncer.vcconfig.DatacenterPaths[0], err)
		t.Fatal(err)
		return
	}

	datastoreObj, err := dc[0].GetDatastoreByName(ctx, sharedDatastore)
	if err != nil {
		t.Errorf("Failed to get datastore with name: %s. Error: %v", sharedDatastore, err)
		t.Fatal(err)
		return
	}
	var dsList []vimtypes.ManagedObjectReference
	dsList = append(dsList, datastoreObj.Reference())

	// Create a test volume
	createSpec := cnstypes.CnsVolumeCreateSpec{
		DynamicData: vimtypes.DynamicData{},
		Name:        testVolumeName,
		VolumeType:  testVolumeType,
		Datastores:  dsList,
		Metadata: cnstypes.CnsVolumeMetadata{
			DynamicData: vimtypes.DynamicData{},
			ContainerCluster: cnstypes.CnsContainerCluster{
				ClusterType: string(cnstypes.CnsClusterTypeKubernetes),
				ClusterId:   metadataSyncer.cfg.Global.ClusterID,
				VSphereUser: metadataSyncer.cfg.VirtualCenter[metadataSyncer.vcconfig.Host].User,
			},
		},
		BackingObjectDetails: &cnstypes.CnsBackingObjectDetails{
			CapacityInMb: gbInMb,
		},
	}

	volumeID, err := cspVolumeManager.CreateVolume(&createSpec)
	if err != nil {
		t.Fatal(err)
	}

	// Set volume id to be queried
	queryFilter := cnstypes.CnsQueryFilter{
		VolumeIds: []cnstypes.CnsVolumeId{
			{
				Id: volumeID.Id,
			},
		},
	}

	// Verify if volume is created
	queryResult, err := metadataSyncer.vcenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) == 0 || queryResult.Volumes[0].VolumeId.Id != volumeID.Id {
		t.Fatalf("Failed to find the newly created volume with ID: %s", volumeID)
	}

	// Set old and new labels
	var oldLabel map[string]string
	var newLabel map[string]string

	oldLabel = make(map[string]string)
	newLabel = make(map[string]string)
	newLabel[testPVLabelName] = testPVLabelValue

	// Test pvUpdate workflow for dynamic provisioning of Volume
	oldPv := getCSPPersistentVolumeSpec(volumeID.Id, v1.PersistentVolumeReclaimRetain, oldLabel, v1.VolumeAvailable)
	newPv := getCSPPersistentVolumeSpec(volumeID.Id, v1.PersistentVolumeReclaimRetain, newLabel, v1.VolumeAvailable)

	pvUpdated(oldPv, newPv, metadataSyncer)

	// Verify pv label of volume matches that of updated metadata
	if queryResult, err = metadataSyncer.vcenter.QueryVolume(ctx, queryFilter); err != nil {
		t.Fatal(err)
	}
	if err = verifyUpdateOperation(queryResult, volumeID.Id, PV); err != nil {
		t.Fatal(err)
	}
	// Delete volume with DeleteDisk=false
	err = cspVolumeManager.DeleteVolume(volumeID.Id, false)

	// Create PV on K8S with VolumeHandle of recently deleted Volume
	pv := getCSPPersistentVolumeSpec(volumeID.Id, v1.PersistentVolumeReclaimRetain, nil, v1.VolumeAvailable)
	if pv, err = k8sclient.CoreV1().PersistentVolumes().Create(pv); err != nil {
		t.Fatal(err)
	}

	// Test pvUpdate workflow on VC for static provisioning of Volume
	// pvUpdate should create the volume on vc for static provisioning
	oldPv = getCSPPersistentVolumeSpec(volumeID.Id, v1.PersistentVolumeReclaimRetain, oldLabel, v1.VolumePending)
	newPv = getCSPPersistentVolumeSpec(volumeID.Id, v1.PersistentVolumeReclaimRetain, newLabel, v1.VolumeAvailable)

	pvUpdated(oldPv, newPv, metadataSyncer)

	// Verify pv label of volume matches that of updated metadata
	if queryResult, err = metadataSyncer.vcenter.QueryVolume(ctx, queryFilter); err != nil {
		t.Fatal(err)
	}
	if err = verifyUpdateOperation(queryResult, volumeID.Id, PV); err != nil {
		t.Fatal(err)
	}

	// Create PVC on K8S to bound to recently created PV
	namespace := testNamespace
	newPVCLabel := make(map[string]string)
	newPVCLabel[testPVCLabelName] = testPVCLabelValue
	pvc := getPersistentVolumeClaimSpec(namespace, newLabel)
	if pvc, err = k8sclient.CoreV1().PersistentVolumeClaims(namespace).Create(pvc); err != nil {
		t.Fatal(err)
	}

	// Test pvcUpdate workflow on VC
	oldPvc := getPersistentVolumeClaimSpec(testNamespace, oldLabel)
	newPvc := getPersistentVolumeClaimSpec(testNamespace, newPVCLabel)
	pvcUpdated(oldPvc, newPvc, metadataSyncer)

	// Verify pvc label of volume matches that of updated metadata
	if queryResult, err = metadataSyncer.vcenter.QueryVolume(ctx, queryFilter); err != nil {
		t.Fatal(err)
	}
	if err = verifyUpdateOperation(queryResult, volumeID.Id, PVC); err != nil {
		t.Fatal(err)
	}

	// Test pvcDelete workflow
	pvcDeleted(newPvc, metadataSyncer)
	if queryResult, err = metadataSyncer.vcenter.QueryVolume(ctx, queryFilter); err != nil {
		t.Fatal(err)
	}

	// Test pvDelete workflow
	pvDeleted(newPv, metadataSyncer)

	// Verify PV has been deleted
	if queryResult, err = metadataSyncer.vcenter.QueryVolume(ctx, queryFilter); err != nil {
		t.Fatal(err)
	}
	if len(queryResult.Volumes) != 0 {
		t.Fatalf("Volume should not exist after deletion with ID: %s", volumeID.Id)
	}
}

// verifyUpdateOperation verifies if an update operation was successful
func verifyUpdateOperation(queryResult *cnstypes.CnsQueryResult, volumeID string, resourceType string) error {
	if len(queryResult.Volumes) == 0 || len(queryResult.Volumes[0].Metadata.EntityMetadata) == 0 || len(queryResult.Volumes[0].Metadata.EntityMetadata[0].GetCnsEntityMetadata().Labels) == 0 {
		return fmt.Errorf("update operation failed for volume Id %s and resource type %s", volumeID, resourceType)
	}
	Labels := queryResult.Volumes[0].Metadata.EntityMetadata[0].GetCnsEntityMetadata().Labels

	for _, query := range Labels {
		queryLabel := query.Key
		queryValue := query.Value
		if resourceType == PVC && queryLabel == testPVCLabelName && queryValue == testPVCLabelValue {
			return nil
		}
		if resourceType == PV && queryLabel == testPVLabelName && queryValue == testPVLabelValue {
			return nil
		}
	}
	return fmt.Errorf("update operation failed for volume Id: %s and resource type %s", volumeID, resourceType)
}

// getCSPPersistentVolumeSpec creates PV volume spec with given Volume Handle, Reclaim Policy, Labels and Phase
func getCSPPersistentVolumeSpec(volumeHandle string, persistentVolumeReclaimPolicy v1.PersistentVolumeReclaimPolicy, labels map[string]string, phase v1.PersistentVolumePhase) *v1.PersistentVolume {
	var pv *v1.PersistentVolume
	var claimRef *v1.ObjectReference
	pv = &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              testVolumeName,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("2Gi"),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       service.Name,
					VolumeHandle: volumeHandle,
					ReadOnly:     false,
					FSType:       "ext4",
				},
			},
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			ClaimRef:                      claimRef,
			PersistentVolumeReclaimPolicy: persistentVolumeReclaimPolicy,
			StorageClassName:              "",
		},
		Status: v1.PersistentVolumeStatus{},
	}
	if labels != nil {
		pv.Labels = labels
	}
	if &phase != nil {
		pv.Status.Phase = phase
	}
	return pv
}

// getPersistentVolumeClaimSpec gets vsphere persistent volume spec with given selector labels.
func getPersistentVolumeClaimSpec(namespace string, labels map[string]string) *v1.PersistentVolumeClaim {
	var (
		pvc *v1.PersistentVolumeClaim
	)
	sc := ""
	pvc = &v1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pvc",
			Namespace:         namespace,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse("1Gi"),
				},
			},
			VolumeName:       testVolumeName,
			StorageClassName: &sc,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: "Bound",
		},
	}
	if labels != nil {
		pvc.Labels = labels
		pvc.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	}

	return pvc
}
