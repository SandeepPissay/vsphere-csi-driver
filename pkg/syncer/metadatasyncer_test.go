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
	"log"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
	"testing"

	"github.com/vmware/govmomi/simulator"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	cnssim "gitlab.eng.vmware.com/hatchway/common-csp/cns/simulator"
	cnstypes "gitlab.eng.vmware.com/hatchway/common-csp/cns/types"
	"gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume"
	cnsvolumetypes "gitlab.eng.vmware.com/hatchway/common-csp/pkg/volume/types"
	cnsvsphere "gitlab.eng.vmware.com/hatchway/common-csp/pkg/vsphere"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	testVolumeName  = "test-volume"
	testClusterName = "test-cluster"
	pvName          = "pv_name"
	gbInMb          = 1024
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

	sharedDatastore := simulator.Map.Any("Datastore").(*simulator.Datastore)
	cfg.Global.Datastore = sharedDatastore.Name

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

func TestMetadataSyncPVWorkflows(t *testing.T) {
	config, cleanup := configFromEnvOrSim()
	defer cleanup()

	// CNS based CSI requires a valid cluster name
	config.Global.ClusterID = testClusterName

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Init VC configuration
	cnsVCenterConfig, err := block.GetVirtualCenterConfig(config)
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

	// Create spec for new volume
	datastoreName := config.Global.Datastore

	dc, err := cspVirtualCenter.GetDatacenter(ctx, cnsVCenterConfig.DatacenterPaths[0])
	if err != nil {
		t.Errorf("Failed to get datacenter for the path: %s. Error: %v", cnsVCenterConfig.DatacenterPaths[0], err)
		t.Fatal(err)
		return
	}

	datastoreObj, err := dc.GetDatastoreByName(ctx, datastoreName)
	if err != nil {
		t.Errorf("Failed to get datastore with name: %s. Error: %v", datastoreName, err)
		t.Fatal(err)
		return
	}
	var dsList []vimtypes.ManagedObjectReference
	dsList = append(dsList, datastoreObj.Reference())

	var metadataSyncer *metadataSyncInformer
	metadataSyncer = &metadataSyncInformer{
		cfg:                  config,
		cnsconfig:            cnsVCenterConfig,
		virtualcentermanager: cspVirtualCenterManager,
		vcenter:              cspVirtualCenter,
	}
	// Create a test volume
	createSpec := cnsvolumetypes.CreateSpec{
		Name:       testVolumeName,
		Datastores: dsList,
		BackingInfo: &cnsvolumetypes.BlockBackingInfo{
			BackingObjectInfo: cnsvolumetypes.BackingObjectInfo{
				Capacity: gbInMb,
			},
		},
		ContainerCluster: cnsvolumetypes.ContainerCluster{
			ClusterID:   config.Global.ClusterID,
			ClusterType: cnstypes.CnsClusterTypeKubernetes,
			VSphereUser: config.VirtualCenter[cnsVCenterConfig.Host].User,
		},
	}

	volumeId, err := cspVolumeManager.CreateVolume(&createSpec)
	if err != nil {
		t.Errorf("Failed to create volume. Error: %+v", err)
		t.Fatal(err)
		return
	}

	// Set volume id to be queried
	queryFilter := cnstypes.CnsQueryFilter{
		VolumeIds: []cnstypes.CnsVolumeId{
			cnstypes.CnsVolumeId{
				Id: volumeId.ID,
			},
		},
	}

	// Verify if volume is created
	queryResult, err := cspVirtualCenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 1 && queryResult.Volumes[0].VolumeId.Id != volumeId.ID {
		t.Fatalf("Failed to find the newly created volume with ID: %s", volumeId)
	}

	var oldLabel map[string]string
	var newLabel map[string]string

	oldLabel = make(map[string]string)
	newLabel = make(map[string]string)
	newLabel[pvName] = queryResult.Volumes[0].Name

	oldPv := getCSPPersistentVolumeSpec(volumeId.ID, v1.PersistentVolumeReclaimRetain, oldLabel)
	newPv := getCSPPersistentVolumeSpec(volumeId.ID, v1.PersistentVolumeReclaimRetain, newLabel)

	pvUpdated(oldPv, newPv, metadataSyncer)

	// Verify pv label of volume matches that of updated metadata
	queryResult, err = cspVirtualCenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}
	queryLabel := queryResult.Volumes[0].Metadata.EntityMetadata[0].GetCnsEntityMetadata().Labels[0].Value
	if len(queryResult.Volumes) != 1 || queryLabel != testVolumeName {
		t.Fatalf("update query failed with : %s %s", volumeId, queryLabel)
	}

	pvDeleted(newPv, metadataSyncer)

	// Verify PV has been deleted
	queryResult, err = cspVirtualCenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 0 {
		t.Fatalf("Volume should not exist after deletion with ID: %s", volumeId.ID)
	}
}

// function to create PV volume spec with given Volume Handle, Reclaim Policy and labels
func getCSPPersistentVolumeSpec(volumeHandle string, persistentVolumeReclaimPolicy v1.PersistentVolumeReclaimPolicy, labels map[string]string) *v1.PersistentVolume {
	var pv *v1.PersistentVolume
	var claimRef *v1.ObjectReference

	pv = &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: testVolumeName,
		},
		Spec: v1.PersistentVolumeSpec{

			PersistentVolumeReclaimPolicy: persistentVolumeReclaimPolicy,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("2Gi"),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			ClaimRef: claimRef,
			PersistentVolumeSource: v1.PersistentVolumeSource{
				GCEPersistentDisk: nil,
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       csidriver,
					VolumeHandle: volumeHandle,
					ReadOnly:     false,
					FSType:       "ext4",
				},
			},
		},
		Status: v1.PersistentVolumeStatus{
			Phase: v1.VolumeAvailable,
		},
	}
	if labels != nil {
		pv.Labels = labels
	}
	return pv
}
