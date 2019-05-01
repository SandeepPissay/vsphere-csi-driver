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
	"context"
	"crypto/tls"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"

	cnssim "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/simulator"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	cspvolume "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/volume"
	cnsvsphere "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vsphere"

	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/block"
)

const (
	testVolumeName  = "test-volume"
	testClusterName = "test-cluster"
)

// configFromSim starts a vcsim instance and returns config for use against the vcsim instance.
// The vcsim instance is configured with an empty tls.Config.
func configFromSim() (*config.Config, func()) {
	return configFromSimWithTLS(new(tls.Config), true)
}

// configFromSimWithTLS starts a vcsim instance and returns config for use against the vcsim instance.
// The vcsim instance is configured with a tls.Config. The returned client
// config can be configured to allow/decline insecure connections.
func configFromSimWithTLS(tlsConfig *tls.Config, insecureAllowed bool) (*config.Config, func()) {
	cfg := &config.Config{}
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

	// PBM Service simulator
	model.Service.RegisterSDK(pbmsim.New())

	sharedDatastore := simulator.Map.Any("Datastore").(*simulator.Datastore)
	cfg.Global.Datastore = sharedDatastore.Name

	cfg.Global.InsecureFlag = insecureAllowed

	cfg.Global.VCenterIP = s.URL.Hostname()
	cfg.Global.VCenterPort = s.URL.Port()
	cfg.Global.User = s.URL.User.Username()
	cfg.Global.Password, _ = s.URL.User.Password()
	cfg.Global.Datacenters = "DC0"
	cfg.VirtualCenter = make(map[string]*config.VirtualCenterConfig)
	cfg.VirtualCenter[s.URL.Hostname()] = &config.VirtualCenterConfig{
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

func configFromEnvOrSim() (*config.Config, func()) {
	cfg := &config.Config{}
	if err := config.FromEnv(cfg); err != nil {
		return configFromSim()
	}
	return cfg, func() {}
}

type FakeNodeManager struct {
	client              *vim25.Client
	sharedDatastoreName string
}

func (f *FakeNodeManager) Initialize(serviceAccount string) error {
	return nil
}

func (f *FakeNodeManager) GetSharedDatastoresInK8SCluster(ctx context.Context) ([]*cnsvsphere.DatastoreInfo, error) {
	finder := find.NewFinder(f.client, false)
	dc, _ := finder.DefaultDatacenter(ctx)
	finder.SetDatacenter(dc)
	ds, _ := finder.Datastore(ctx, f.sharedDatastoreName)

	var dsMo mo.Datastore
	pc := property.DefaultCollector(f.client)
	properties := []string{"info"}
	_ = pc.RetrieveOne(ctx, ds.Reference(), properties, &dsMo)

	return []*cnsvsphere.DatastoreInfo{
		{
			&cnsvsphere.Datastore{
				object.NewDatastore(nil, ds.Reference()),
				nil},
			dsMo.Info.GetDatastoreInfo(),
		},
	}, nil
}

func (f *FakeNodeManager) GetNodeByName(nodeName string) (*cnsvsphere.VirtualMachine, error) {
	return nil, nil
}

type controllerTest struct {
	controller *controller
	config     *config.Config
	vcenter    *cnsvsphere.VirtualCenter
}

var (
	controllerTestInstance *controllerTest
	onceForControllerTest  sync.Once
)

func getControllerTest(t *testing.T) *controllerTest {
	onceForControllerTest.Do(func() {
		config, _ := configFromEnvOrSim()

		// CNS based CSI requires a valid cluster name
		config.Global.ClusterID = testClusterName

		ctx := context.Background()

		vcenterconfig, err := block.GetVirtualCenterConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		vcManager := cnsvsphere.GetVirtualCenterManager()
		vcenter, err := vcManager.RegisterVirtualCenter(vcenterconfig)
		if err != nil {
			t.Fatal(err)
		}

		err = vcenter.Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}

		manager := &block.Manager{
			VcenterConfig:  vcenterconfig,
			CnsConfig:      config,
			VolumeManager:  cspvolume.GetManager(vcenter),
			VcenterManager: cnsvsphere.GetVirtualCenterManager(),
		}
		c := &controller{
			manager: manager,
			nodeMgr: &FakeNodeManager{
				client:              vcenter.Client.Client,
				sharedDatastoreName: config.Global.Datastore,
			},
		}

		controllerTestInstance = &controllerTest{
			controller: c,
			config:     config,
			vcenter:    vcenter,
		}
	})
	return controllerTestInstance
}

func TestCreateVolumeWithStoragePolicy(t *testing.T) {
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct := getControllerTest(t)

	// Create
	params := make(map[string]string, 0)
	params[block.AttributeDiskParentType] = string(block.DatastoreType)
	params[block.AttributeDiskParentName] = ct.config.Global.Datastore

	// PBM simulator defaults
	params[block.AttributeStoragePolicyType] = string(block.StoragePolicyType)
	params[block.AttributeStoragePolicyName] = "vSAN Default Storage Policy"
	if v := os.Getenv("VSPHERE_STORAGE_POLICY_NAME"); v != "" {
		params[block.AttributeStoragePolicyName] = v
	}
	capabilities := []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	reqCreate := &csi.CreateVolumeRequest{
		Name: testVolumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1 * block.GbInBytes,
		},
		Parameters:         params,
		VolumeCapabilities: capabilities,
	}

	respCreate, err := ct.controller.CreateVolume(ctx, reqCreate)
	if err != nil {
		t.Fatal(err)
	}
	volID := respCreate.Volume.VolumeId

	// Varify the volume has been create with corresponding storage policy ID
	pc, err := pbm.NewClient(ctx, ct.vcenter.Client.Client)
	if err != nil {
		t.Fatal(err)
	}

	profileId, err := pc.ProfileIDByName(ctx, params[block.AttributeStoragePolicyName])
	if err != nil {
		t.Fatal(err)
	}

	queryFilter := cnstypes.CnsQueryFilter{
		VolumeIds: []cnstypes.CnsVolumeId{
			{
				Id: volID,
			},
		},
	}
	queryResult, err := ct.vcenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 1 && queryResult.Volumes[0].VolumeId.Id != volID {
		t.Fatalf("Failed to find the newly created volume with ID: %s", volID)
	}

	if queryResult.Volumes[0].StoragePolicyId != profileId {
		t.Fatalf("Failed to match volume policy ID: %s", profileId)
	}

	// Delete
	reqDelete := &csi.DeleteVolumeRequest{
		VolumeId: volID,
	}
	_, err = ct.controller.DeleteVolume(ctx, reqDelete)
	if err != nil {
		t.Errorf("DeleteVolume failed: %v", err)
	}

	// Varify the volume has been deleted
	queryResult, err = ct.vcenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 0 {
		t.Fatalf("Volume should not exist after deletion with ID: %s", volID)
	}
}

func TestCompleteControllerFlow(t *testing.T) {
	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct := getControllerTest(t)

	// Create
	params := make(map[string]string, 0)
	params[block.AttributeDiskParentType] = string(block.DatastoreType)
	params[block.AttributeDiskParentName] = ct.config.Global.Datastore
	capabilities := []*csi.VolumeCapability{
		{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}

	reqCreate := &csi.CreateVolumeRequest{
		Name: testVolumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1 * block.GbInBytes,
		},
		Parameters:         params,
		VolumeCapabilities: capabilities,
	}

	respCreate, err := ct.controller.CreateVolume(ctx, reqCreate)
	if err != nil {
		t.Fatal(err)
	}
	volID := respCreate.Volume.VolumeId

	// Varify the volume has been created
	queryFilter := cnstypes.CnsQueryFilter{
		VolumeIds: []cnstypes.CnsVolumeId{
			{
				Id: volID,
			},
		},
	}
	queryResult, err := ct.vcenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 1 && queryResult.Volumes[0].VolumeId.Id != volID {
		t.Fatalf("Failed to find the newly created volume with ID: %s", volID)
	}

	// Delete
	reqDelete := &csi.DeleteVolumeRequest{
		VolumeId: volID,
	}
	_, err = ct.controller.DeleteVolume(ctx, reqDelete)
	if err != nil {
		t.Errorf("DeleteVolume failed: %v", err)
	}

	// Varify the volume has been deleted
	queryResult, err = ct.vcenter.QueryVolume(ctx, queryFilter)
	if err != nil {
		t.Fatal(err)
	}

	if len(queryResult.Volumes) != 0 {
		t.Fatalf("Volume should not exist after deletion with ID: %s", volID)
	}
}
