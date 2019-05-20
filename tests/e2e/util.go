package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vim25types "github.com/vmware/govmomi/vim25/types"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"strings"
	"time"
)

// getVSphereStorageClassSpec returns Storage Class with supplied storage class parameters
func getVSphereStorageClassSpec(name string, scParameters map[string]string) *storagev1.StorageClass {
	var sc *storagev1.StorageClass

	sc = &storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: "vsphere.csi.vmware.com",
	}
	if scParameters != nil {
		sc.Parameters = scParameters
	}
	return sc
}

// getPvFromClaim returns PersistentVolume for requested claim
func getPvFromClaim(client clientset.Interface, namespace string, claimName string) *v1.PersistentVolume {
	pvclaim, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(claimName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	pv, err := client.CoreV1().PersistentVolumes().Get(pvclaim.Spec.VolumeName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return pv
}

// getNodeUUID returns Node VM UUID for requested node
func getNodeUUID(client clientset.Interface, nodeName string) string {
	node, err := client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimPrefix(node.Spec.ProviderID, ProviderPrefix)
}

// verifyVolumeMetadataInCNS verifies container volume metadata is matching the one is CNS cache
func verifyVolumeMetadataInCNS(vs *VSphere, volumeId string, PersistentVolumeClaimName string, PersistentVolumeName string, PodName string) error {
	queryResult, err := vs.queryCNSVolumeWithResult(volumeId)
	if err != nil {
		return err
	}
	if len(queryResult.Volumes) != 1 || queryResult.Volumes[0].VolumeId.Id != volumeId {
		return fmt.Errorf("Failed to query cns volume %s", volumeId)
	}
	for _, metadata := range queryResult.Volumes[0].Metadata.EntityMetadata {
		kubernetesMetadata := metadata.(*cnstypes.CnsKubernetesEntityMetadata)
		if kubernetesMetadata.EntityType == "POD" && kubernetesMetadata.EntityName != PodName {
			return fmt.Errorf("entity POD with name %s not found for volume %s", PodName, volumeId)
		} else if kubernetesMetadata.EntityType == "PERSISTENT_VOLUME" && kubernetesMetadata.EntityName != PersistentVolumeName {
			return fmt.Errorf("entity PV with name %s not found for volume %s", PersistentVolumeName, volumeId)
		} else if kubernetesMetadata.EntityType == "PERSISTENT_VOLUME_CLAIM" && kubernetesMetadata.EntityName != PersistentVolumeClaimName {
			return fmt.Errorf("entity PVC with name %s not found for volume %s", PersistentVolumeClaimName, volumeId)
		}
	}
	By(fmt.Sprintf("Verified volume %s successfully", volumeId))
	return nil
}

// isCNSDiskDetached checks if volume is attached with VM whose UUID is supplied as parameter
// This function checks disks status every 3 seconds until detachTimeout, which is set to 360 seconds
func isCNSDiskDetached(vs *VSphere, vmUUID string, volumeID string) (bool, error) {
	var (
		detachTimeout  = 360 * time.Second
		detachPollTime = 3 * time.Second
	)
	err := wait.Poll(detachPollTime, detachTimeout, func() (bool, error) {
		diskAttached, _ := vs.verifyCNSVolumeIsAttached(vmUUID, volumeID)
		if diskAttached == false {
			framework.Logf("Disk - %s successfully detached", volumeID)
			return true, nil
		}
		framework.Logf("Waiting for disk - %s to be detached from the node", volumeID)
		return false, nil
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// getVirtualDeviceByDiskID gets the virtual device by diskID
func getVirtualDeviceByDiskID(ctx context.Context, vm *object.VirtualMachine, diskID string) (vim25types.BaseVirtualDevice, error) {
	vmDevices, err := vm.Device(ctx)
	if err != nil {
		framework.Logf("Failed to get the devices for VM: %q. err: %+v", vm.InventoryPath, err)
		return nil, err
	}
	for _, device := range vmDevices {
		if vmDevices.TypeName(device) == "VirtualDisk" {
			if virtualDisk, ok := device.(*vim25types.VirtualDisk); ok {
				if virtualDisk.VDiskId != nil && virtualDisk.VDiskId.Id == diskID {
					framework.Logf("Found FCDID %q attached to VM %q", diskID, vm.Name())
					return device, nil
				}
			}
		}
	}
	framework.Logf("Failed to find FCDID %q attached to VM %q", diskID, vm.Name())
	return nil, nil
}
