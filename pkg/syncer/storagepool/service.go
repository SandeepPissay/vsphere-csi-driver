/*
Copyright 2020 VMware, Inc.

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

package storagepool

import (
	"context"
	"reflect"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/vsphere-csi-driver/pkg/csi/service/logger"
	spv1alpha1 "sigs.k8s.io/vsphere-csi-driver/pkg/syncer/storagepool/apis/cns/v1alpha1"
	commontypes "sigs.k8s.io/vsphere-csi-driver/pkg/syncer/types"
)

// InitStoragePoolService initializes the StoragePool service that updates
// vSphere Datastore information into corresponding k8s StoragePool resources.
func InitStoragePoolService(ctx context.Context, configInfo *commontypes.ConfigInfo) error {
	log := logger.GetLogger(ctx)
	log.Infof("Initializing Storage Pool Service")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Errorf("Failed to get Kubernetes config. Err: %+v", err)
		return err
	}

	apiextensionsClientSet, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		log.Errorf("Failed to create Kubernetes client using config. Err: %+v", err)
		return err
	}

	// Create StoragePool CRD
	crdKind := reflect.TypeOf(spv1alpha1.StoragePool{}).Name()
	err = createCustomResourceDefinition(ctx, apiextensionsClientSet, "storagepools", crdKind)
	if err != nil {
		log.Errorf("Failed to create %q CRD. Err: %+v", crdKind, err)
		return err
	}

	// Get VC connection
	vc, err := commontypes.GetVirtualCenterInstance(ctx, configInfo)
	if err != nil {
		log.Errorf("Failed to get vCenter from ConfigInfo. Err: %+v", err)
		return err
	}

	// Start the listeners
	err = InitHostMountListener(ctx, vc, configInfo.Cfg.Global.ClusterID)
	if err != nil {
		log.Errorf("Failed starting the HostMount listener. Err: %+v", err)
	}

	err = InitDatastoreCapacityListener(ctx, vc, configInfo.Cfg.Global.ClusterID)
	if err != nil {
		log.Errorf("Failed starting the DatastoreCapacity listener. Err: %+v", err)
	}

	log.Infof("Done initializing Storage Pool Service")
	return nil
}