package e2e

import (
	"context"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vim25types "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"strings"
)

// getVSphereStorageClassSpec returns Storage Class Spec with supplied storage class parameters
func getVSphereStorageClassSpec(scName string, scParameters map[string]string) *storagev1.StorageClass {
	var sc *storagev1.StorageClass
	sc = &storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "sc-",
		},
		Provisioner: e2evSphereCSIBlockDriverName,
	}
	// If scName is specified, use that name, else auto-generate storage class name
	if scName != "" {
		sc.ObjectMeta.Name = scName
	}
	if scParameters != nil {
		sc.Parameters = scParameters
	}
	return sc
}

// getPvFromClaim returns PersistentVolume for requested claim
func getPvFromClaim(client clientset.Interface, namespace string, claimName string) *corev1.PersistentVolume {
	pvclaim, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(claimName, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	pv, err := client.CoreV1().PersistentVolumes().Get(pvclaim.Spec.VolumeName, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return pv
}

// getNodeUUID returns Node VM UUID for requested node
func getNodeUUID(client clientset.Interface, nodeName string) string {
	node, err := client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return strings.TrimPrefix(node.Spec.ProviderID, providerPrefix)
}

// verifyVolumeMetadataInCNS verifies container volume metadata is matching the one is CNS cache
func verifyVolumeMetadataInCNS(vs *vSphere, volumeID string, PersistentVolumeClaimName string, PersistentVolumeName string, PodName string) error {
	queryResult, err := vs.queryCNSVolumeWithResult(volumeID)
	if err != nil {
		return err
	}
	if len(queryResult.Volumes) != 1 || queryResult.Volumes[0].VolumeId.Id != volumeID {
		return fmt.Errorf("Failed to query cns volume %s", volumeID)
	}
	for _, metadata := range queryResult.Volumes[0].Metadata.EntityMetadata {
		kubernetesMetadata := metadata.(*cnstypes.CnsKubernetesEntityMetadata)
		if kubernetesMetadata.EntityType == "POD" && kubernetesMetadata.EntityName != PodName {
			return fmt.Errorf("entity POD with name %s not found for volume %s", PodName, volumeID)
		} else if kubernetesMetadata.EntityType == "PERSISTENT_VOLUME" && kubernetesMetadata.EntityName != PersistentVolumeName {
			return fmt.Errorf("entity PV with name %s not found for volume %s", PersistentVolumeName, volumeID)
		} else if kubernetesMetadata.EntityType == "PERSISTENT_VOLUME_CLAIM" && kubernetesMetadata.EntityName != PersistentVolumeClaimName {
			return fmt.Errorf("entity PVC with name %s not found for volume %s", PersistentVolumeClaimName, volumeID)
		}
	}
	ginkgo.By(fmt.Sprintf("successfully verified metadata of the volume %q", volumeID))
	return nil
}

// getVirtualDeviceByDiskID gets the virtual device by diskID
func getVirtualDeviceByDiskID(ctx context.Context, vm *object.VirtualMachine, diskID string) (vim25types.BaseVirtualDevice, error) {
	vmname, err := vm.Common.ObjectName(ctx)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	vmDevices, err := vm.Device(ctx)
	if err != nil {
		framework.Logf("Failed to get the devices for VM: %q. err: %+v", vmname, err)
		return nil, err
	}
	for _, device := range vmDevices {
		if vmDevices.TypeName(device) == "VirtualDisk" {
			if virtualDisk, ok := device.(*vim25types.VirtualDisk); ok {
				if virtualDisk.VDiskId != nil && virtualDisk.VDiskId.Id == diskID {
					framework.Logf("Found FCDID %q attached to VM %q", diskID, vmname)
					return device, nil
				}
			}
		}
	}
	framework.Logf("Failed to find FCDID %q attached to VM %q", diskID, vmname)
	return nil, nil
}

// getPersistentVolumeClaimSpecWithStorageClass return the PersistentVolumeClaim spec with specified storage class
func getPersistentVolumeClaimSpecWithStorageClass(namespace string, diskSize string, storageclass *storagev1.StorageClass, pvclaimlabels map[string]string) *corev1.PersistentVolumeClaim {
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pvc-",
			Namespace:    namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(diskSize),
				},
			},
			StorageClassName: &(storageclass.Name),
		},
	}

	if pvclaimlabels != nil {
		claim.Labels = pvclaimlabels
	}
	return claim
}

// createPVCAndStorageClass helps creates a storage class with specified name, storageclass parameters and PVC using storage class
func createPVCAndStorageClass(client clientset.Interface, pvcnamespace string, pvclaimlabels map[string]string, scParameters map[string]string, ds string) (*storagev1.StorageClass, *corev1.PersistentVolumeClaim, error) {
	ginkgo.By(fmt.Sprintf("Creating StorageClass With scParameters: %+v", scParameters))
	storageclass, err := client.StorageV1().StorageClasses().Create(getVSphereStorageClassSpec("", scParameters))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to create storage class with err: %v", err))

	ginkgo.By("Creating PVC using the StorageClass")
	disksize := diskSize
	if ds != "" {
		disksize = ds
	}
	pvcspec := getPersistentVolumeClaimSpecWithStorageClass(pvcnamespace, disksize, storageclass, pvclaimlabels)
	pvclaim, err := framework.CreatePVC(client, pvcnamespace, pvcspec)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return storageclass, pvclaim, err
}

// getLabelsMapFromKeyValue returns map[string]string for given array of vim25types.KeyValue
func getLabelsMapFromKeyValue(labels []vim25types.KeyValue) map[string]string {
	labelsMap := make(map[string]string)
	for _, label := range labels {
		labelsMap[label.Key] = label.Value
	}
	return labelsMap
}
