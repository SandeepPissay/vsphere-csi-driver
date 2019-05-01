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
	csictx "github.com/rexray/gocsi/context"
	"k8s.io/klog"
	"os"
	cnsconfig "sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	vTypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
	k8s "sigs.k8s.io/vsphere-csi-driver/pkg/kubernetes"
)

type metadataSyncInformer struct {
	cfg                *cnsconfig.Config
	k8sInformerManager *k8s.InformerManager
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
	// Create the kubernetes client from config
	k8sclient, err := k8s.NewClient(metadataSyncer.cfg.Global.ServiceAccount)
	if err != nil {
		klog.Errorf("Creating Kubernetes client failed. Err: %v", err)
		return err
	}
	// Set up kubernetes resource listeners for metadata syncer
	metadataSyncer.k8sInformerManager = k8s.NewInformer(k8sclient)
	metadataSyncer.k8sInformerManager.AddPVCListener(nil, PVCUpdated, PVCDeleted)
	metadataSyncer.k8sInformerManager.AddPVListener(nil, PVUpdated, PVDeleted)
	metadataSyncer.k8sInformerManager.AddPodListener(nil, PodUpdated, PodDeleted)
	klog.V(2).Infof("Initialized metadata syncer")
	stopch := metadataSyncer.k8sInformerManager.Listen()
	<-(stopch)
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

func PVUpdated(oldObj, newObj interface{}) {
	fmt.Printf("Temporary implementation of PV Update\n")
}

func PVDeleted(obj interface{}) {
	fmt.Printf("Temporary implementation of PV Delete\n")
}

func PodUpdated(oldObj, newObj interface{}) {
	fmt.Printf("Temporary implementation of Pod Update\n")
}

func PodDeleted(obj interface{}) {
	fmt.Printf("Temporary implementation of Pod Delete\n")
}
