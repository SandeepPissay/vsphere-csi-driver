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

package kubernetes

import (
	"k8s.io/api/core/v1"
	"os"

	"k8s.io/klog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClient creates a newk8s client based on a service account
func NewClient(name string) (clientset.Interface, error) {
	kubecfgPath := os.Getenv(EnvKubeConfig)
	if kubecfgPath == "*" {
		kubecfgPath = DefaultKubeConfigPath
	}

	var config *restclient.Config
	var err error
	if kubecfgPath != "" {
		klog.V(2).Info("k8s client using kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubecfgPath)
		if err != nil {
			klog.Errorf("BuildConfigFromFlags failed %q", err)
			return nil, err
		}
	} else {
		klog.V(2).Info("k8s client using in-cluster config")
		config, err = restclient.InClusterConfig()
		if err != nil {
			klog.Errorf("InClusterConfig failed %q", err)
			return nil, err
		}
	}

	newConfig := restclient.AddUserAgent(config, name)

	return clientset.NewForConfig(newConfig)
}

// GetAllNodes returns all kubernetes nodes registered with the API server
func GetAllNodes(k8sclient clientset.Interface) (*v1.NodeList, error) {
	return k8sclient.CoreV1().Nodes().List(metav1.ListOptions{})
}
