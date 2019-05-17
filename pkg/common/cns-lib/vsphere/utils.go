package vsphere

import (
	"errors"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"sigs.k8s.io/vsphere-csi-driver/pkg/common/config"
	"strconv"
	"strings"
)

// IsInvalidCredentialsError returns true if error is of type InvalidLogin
func IsInvalidCredentialsError(err error) bool {
	isInvalidCredentialsError := false
	if soap.IsSoapFault(err) {
		_, isInvalidCredentialsError = soap.ToSoapFault(err).VimFault().(types.InvalidLogin)
	}
	return isInvalidCredentialsError
}

// GetCnsKubernetesEntityMetaData creates a CnsKubernetesEntityMetadataObject object from given parameters
func GetCnsKubernetesEntityMetaData(entityName string, labels map[string]string, deleteFlag bool, entityType string, namespace string) *cnstypes.CnsKubernetesEntityMetadata {
	// Create new metadata spec
	var newLabels []types.KeyValue
	for labelKey, labelVal := range labels {
		newLabels = append(newLabels, types.KeyValue{
			Key:   labelKey,
			Value: labelVal,
		})
	}

	entityMetadata := &cnstypes.CnsKubernetesEntityMetadata{}
	entityMetadata.EntityName = entityName
	entityMetadata.Delete = deleteFlag
	if labels != nil {
		entityMetadata.Labels = newLabels
	}
	entityMetadata.EntityType = entityType
	entityMetadata.Namespace = namespace
	return entityMetadata
}

// GetContainerCluster creates ContainerCluster object from given parameters
func GetContainerCluster(clusterid string, username string) cnstypes.CnsContainerCluster {
	return cnstypes.CnsContainerCluster{
		ClusterType: string(cnstypes.CnsClusterTypeKubernetes),
		ClusterId:   clusterid,
		VSphereUser: username,
	}

}

// GetVirtualCenterConfig returns VirtualCenterConfig Object created using vSphere Configuration
// specified in the argurment.
func GetVirtualCenterConfig(cfg *config.Config) (*VirtualCenterConfig, error) {
	var err error
	vCenterIPs, err := GetVcenterIPs(cfg) //  make([]string, 0)
	if err != nil {
		return nil, err
	}
	host := vCenterIPs[0]
	port, err := strconv.Atoi(cfg.VirtualCenter[host].VCenterPort)
	if err != nil {
		return nil, err
	}
	vcConfig := &VirtualCenterConfig{
		Host:            host,
		Port:            port,
		Username:        cfg.VirtualCenter[host].User,
		Password:        cfg.VirtualCenter[host].Password,
		Insecure:        cfg.VirtualCenter[host].InsecureFlag,
		DatacenterPaths: strings.Split(cfg.VirtualCenter[host].Datacenters, ","),
	}
	return vcConfig, nil
}

// GetVcenterIPs returns list of vCenter IPs from VSphereConfig
func GetVcenterIPs(cfg *config.Config) ([]string, error) {
	var err error
	vCenterIPs := make([]string, 0)
	for key := range cfg.VirtualCenter {
		vCenterIPs = append(vCenterIPs, key)
	}
	if len(vCenterIPs) == 0 {
		err = errors.New("Unable get vCenter Hosts from VSphereConfig")
	}
	return vCenterIPs, err
}
