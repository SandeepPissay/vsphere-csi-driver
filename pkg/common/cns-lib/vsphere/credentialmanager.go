// Copyright 2018 VMware, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vsphere

import (
	"errors"
	"k8s.io/klog"
	"sync"
)

type Credential struct {
	User     string
	Password string
}

type CredentialStore interface {
	GetCredential(server string) (*Credential, error)
}

type CredentialManager interface {
	SetCredentialStore(credentialStore CredentialStore)
	GetCredentialStore() (CredentialStore, error)
}

var (
	ErrCredentialStoreNotSet = errors.New("credential store not set")
)
var (
	// vcManagerInst is a VirtualCenterManager singleton.
	credentialManager *defaultCredentialManager
	// onceForVCManager is used for initializing the VirtualCenterManager singleton.
	onceForCredManager sync.Once
)

// GetCredentialManager returns the VirtualCenterManager singleton.
func GetCredentialManager() CredentialManager {
	onceForCredManager.Do(func() {
		klog.V(1).Info("Initializing defaultCredentialManager...")
		credentialManager = &defaultCredentialManager{}
		klog.V(1).Info("Successfully initialized defaultCredentialManager")
	})
	return credentialManager
}

// defaultVirtualCenterManager holds virtual center information and provides
// functionality around it.
type defaultCredentialManager struct {
	// virtualCenters map hosts to *VirtualCenter instances.
	credentialStore CredentialStore
}

func (m *defaultCredentialManager) SetCredentialStore(credentialStore CredentialStore) {
	m.credentialStore = credentialStore
}

func (m *defaultCredentialManager) GetCredentialStore() (CredentialStore, error) {
	if m.credentialStore == nil {
		return nil, ErrCredentialStoreNotSet
	}
	return m.credentialStore, nil
}
