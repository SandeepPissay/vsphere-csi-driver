/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VirtualMachineServiceFinalizer string = "virtualmachineservice.vmoperator.vmware.com"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineService
// +k8s:openapi-gen=true
// +resource:path=virtualmachineservices,strategy=VirtualMachineServiceStrategy
type VirtualMachineService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineServiceSpec   `json:"spec,omitempty"`
	Status VirtualMachineServiceStatus `json:"status,omitempty"`
}

type VirtualMachineServicePort struct {
	Name string `json:"name"`

	// The IP protocol for this port.  Supports "TCP", "UDP", and "SCTP".
	//Protocol corev1.Protocol
	Protocol string `json:"protocol"`

	// The port that will be exposed on the service.
	Port int32 `json:"port"`

	TargetPort int32 `json:"targetPort"`
}

// VirtualMachineService Type string describes ingress methods for a service
type VirtualMachineServiceType string

// These types correspond to a subset of the core Service Types
const (
	// VirtualMachineServiceTypeClusterIP means a service will only be accessible inside the
	// cluster, via the cluster IP.
	VirtualMachineServiceTypeClusterIP VirtualMachineServiceType = "ClusterIP"

	// VirtualMachineServiceTypeLoadBalancer means a service will be exposed via an
	// external load balancer (if the cloud provider supports it), in addition
	// to 'NodePort' type.
	VirtualMachineServiceTypeLoadBalancer VirtualMachineServiceType = "LoadBalancer"

	// VirtualMachineServiceTypeExternalName means a service consists of only a reference to
	// an external name that kubedns or equivalent will return as a CNAME
	// record, with no exposing or proxying of any pods involved.
	VirtualMachineServiceTypeExternalName VirtualMachineServiceType = "ExternalName"
)

// VirtualMachineServiceSpec defines the desired state of VirtualMachineService
type VirtualMachineServiceSpec struct {
	Type     VirtualMachineServiceType   `json:"type"`
	Ports    []VirtualMachineServicePort `json:"ports"`
	Selector map[string]string           `json:"selector"`

	// Just support cluster IP for now
	ClusterIP    string `json:"clusterIp,omitempty"`
	ExternalName string `json:"externalName,omitempty"`
}

// VirtualMachineServiceStatus defines the observed state of VirtualMachineService
type VirtualMachineServiceStatus struct {
	// LoadBalancer contains the current status of the load-balancer,
	// if one is present.
	// +optional
	LoadBalancer LoadBalancerStatus `json:"loadBalancer,omitempty"`
}

// LoadBalancerStatus represents the status of a load-balancer.
type LoadBalancerStatus struct {
	// Ingress is a list containing ingress points for the load-balancer.
	// Traffic intended for the service should be sent to these ingress points.
	// +optional
	Ingress []LoadBalancerIngress `json:"ingress,omitempty"`
}

// LoadBalancerIngress represents the status of a load-balancer ingress point:
// traffic intended for the service should be sent to an ingress point.
type LoadBalancerIngress struct {
	// IP is set for load-balancer ingress points that are IP based
	IP string `json:"ip,omitempty"`

	// Hostname is set for load-balancer ingress points that are DNS based
	Hostname string `json:"hostname,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VirtualMachineServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineService `json:"items"`
}
