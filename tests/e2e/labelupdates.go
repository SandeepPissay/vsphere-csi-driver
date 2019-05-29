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

package e2e

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

/*
   Tests to verify label updates .

   Steps
   - Create StorageClass, PVC and PV with valid parameters
   Test 1) Modify labels on PVC and PV and verify labels are getting updated by metadata syncer.
   Test 2) Delete labels on PVC and PV and verify labels are getting removed by metadata syncer.
           Verify Pod name label is added when Pod is created with container volume.
           Verify Pod name label associated with volume is removed when Pod is deleted.
   Cleanup
   - Delete PVC and StorageClass and verify volume is deleted from CNS.
*/

var _ bool = ginkgo.Describe("[csi-block-e2e] label-updates", func() {
	f := framework.NewDefaultFramework("e2e-volume-label-updates")
	var (
		client     clientset.Interface
		namespace  string
		labelKey   string
		labelValue string
	)
	ginkgo.BeforeEach(func() {
		client = f.ClientSet
		namespace = f.Namespace.Name
		nodeList := framework.GetReadySchedulableNodesOrDie(f.ClientSet)
		if !(len(nodeList.Items) > 0) {
			framework.Failf("Unable to find ready and schedulable Node")
		}
		bootstrap()
		labelKey = "app"
		labelValue = "e2e-labels"
	})

	ginkgo.It("verify labels are created in CNS after updating pvc and/or pv with new labels", func() {
		ginkgo.By(fmt.Sprintf("Invoking test to verify labels creation"))
		sc, pvc, err := createPVCAndStorageClass(client, namespace, nil, nil, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer client.StorageV1().StorageClasses().Delete(sc.Name, nil)
		defer client.CoreV1().PersistentVolumeClaims(namespace).Delete(pvc.Name, nil)

		ginkgo.By(fmt.Sprintf("Waiting for claim %s to be in bound phase", pvc.Name))
		pvs, err := framework.WaitForPVClaimBoundPhase(client, []*v1.PersistentVolumeClaim{pvc}, framework.ClaimProvisionTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pvs).NotTo(gomega.BeEmpty())
		pv := pvs[0]

		labels := make(map[string]string)
		labels[labelKey] = labelValue

		ginkgo.By(fmt.Sprintf("Updating labels %+v for pvc %s in namespace %s", labels, pvc.Name, pvc.Namespace))
		pvc, err = client.CoreV1().PersistentVolumeClaims(namespace).Get(pvc.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		pvc.Labels = labels
		_, err = client.CoreV1().PersistentVolumeClaims(namespace).Update(pvc)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Updating labels %+v for pv %s", labels, pv.Name))
		pv.Labels = labels

		_, err = client.CoreV1().PersistentVolumes().Update(pv)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for labels %+v to be updated for pvc %s in namespace %s", labels, pvc.Name, pvc.Namespace))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, labels, string(cnstypes.CnsKubernetesEntityTypePVC), pvc.Name, pvc.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for labels %+v to be updated for pv %s", labels, pv.Name))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, labels, string(cnstypes.CnsKubernetesEntityTypePV), pv.Name, pv.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Deleting pvc %s in namespace %s", pvc.Name, pvc.Namespace))
		err = client.CoreV1().PersistentVolumeClaims(namespace).Delete(pvc.Name, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for volume %s to be deleted", pv.Spec.CSI.VolumeHandle))
		err = e2eVSphere.waitForCNSVolumeToBeDeleted(pv.Spec.CSI.VolumeHandle)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("verify labels are removed in CNS after removing them from pvc and/or pv", func() {
		ginkgo.By("Invoking test to verify labels deletion")
		labels := make(map[string]string)
		labels[labelKey] = labelValue

		sc, pvc, err := createPVCAndStorageClass(client, namespace, nil, nil, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer client.StorageV1().StorageClasses().Delete(sc.Name, nil)
		defer client.CoreV1().PersistentVolumeClaims(namespace).Delete(pvc.Name, nil)

		ginkgo.By(fmt.Sprintf("Waiting for claim %s to be in bound phase", pvc.Name))
		pvs, err := framework.WaitForPVClaimBoundPhase(client, []*v1.PersistentVolumeClaim{pvc}, framework.ClaimProvisionTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pvs).NotTo(gomega.BeEmpty())
		pv := pvs[0]

		ginkgo.By(fmt.Sprintf("Updating labels %+v for pv %s", labels, pv.Name))
		pv.Labels = labels
		_, err = client.CoreV1().PersistentVolumes().Update(pv)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for labels %+v to be updated for pv %s", labels, pv.Name))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, labels, string(cnstypes.CnsKubernetesEntityTypePV), pv.Name, pv.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Fetching updated pvc %s in namespace %s", pvc.Name, pvc.Namespace))
		pvc, err = client.CoreV1().PersistentVolumeClaims(namespace).Get(pvc.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Deleting labels %+v for pvc %s in namespace %s", labels, pvc.Name, pvc.Namespace))
		pvc.Labels = make(map[string]string)
		_, err = client.CoreV1().PersistentVolumeClaims(namespace).Update(pvc)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for labels %+v to be deleted for pvc %s in namespace %s", labels, pvc.Name, pvc.Namespace))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, pvc.Labels, string(cnstypes.CnsKubernetesEntityTypePVC), pvc.Name, pvc.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Fetching updated pv %s", pv.Name))
		pv, err = client.CoreV1().PersistentVolumes().Get(pv.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Deleting labels %+v for pv %s", labels, pv.Name))
		pv.Labels = make(map[string]string)
		_, err = client.CoreV1().PersistentVolumes().Update(pv)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for labels %+v to be deleted for pv %s", labels, pv.Name))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, pv.Labels, string(cnstypes.CnsKubernetesEntityTypePV), pv.Name, pv.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Deleting pvc %s in namespace %s", pvc.Name, pvc.Namespace))
		err = client.CoreV1().PersistentVolumeClaims(namespace).Delete(pvc.Name, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Waiting for volume %s to be deleted", pv.Spec.CSI.VolumeHandle))
		err = e2eVSphere.waitForCNSVolumeToBeDeleted(pv.Spec.CSI.VolumeHandle)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("verify podname label is created/deleted when pod with cns volume is created/deleted.", func() {
		ginkgo.By(fmt.Sprintf("Invoking test to verify pod name label updates"))
		sc, pvc, err := createPVCAndStorageClass(client, namespace, nil, nil, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer client.StorageV1().StorageClasses().Delete(sc.Name, nil)
		defer client.CoreV1().PersistentVolumeClaims(namespace).Delete(pvc.Name, nil)

		ginkgo.By(fmt.Sprintf("Waiting for claim %s to be in bound phase", pvc.Name))
		pvs, err := framework.WaitForPVClaimBoundPhase(client, []*v1.PersistentVolumeClaim{pvc}, framework.ClaimProvisionTimeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(pvs).NotTo(gomega.BeEmpty())
		pv := pvs[0]

		ginkgo.By("Creating pod")
		pod, err := framework.CreatePod(client, namespace, nil, []*v1.PersistentVolumeClaim{pvc}, false, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		defer client.CoreV1().Pods(namespace).Delete(pod.Name, nil)

		ginkgo.By("Verify volume is attached to the node")
		isDiskAttached, err := e2eVSphere.isVolumeAttachedToNode(client, pv.Spec.CSI.VolumeHandle, pod.Spec.NodeName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(isDiskAttached).To(gomega.BeTrue(), fmt.Sprintf("Volume is not attached to the node"))

		ginkgo.By(fmt.Sprintf("Waiting for pod name to be updated for volume %s by metadata-syncer", pv.Spec.CSI.VolumeHandle))
		err = e2eVSphere.waitForLabelsToBeUpdated(pv.Spec.CSI.VolumeHandle, nil, string(cnstypes.CnsKubernetesEntityTypePOD), pod.Name, pod.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Deleting the pod")
		framework.DeletePodWithWait(f, client, pod)

		ginkgo.By("Verify volume is detached from the node")
		isDiskDetached, err := e2eVSphere.waitForVolumeDetachedFromNode(client, pv.Spec.CSI.VolumeHandle, pod.Spec.NodeName)
		gomega.Expect(isDiskDetached).To(gomega.BeTrue(), fmt.Sprintf("Volume %q is not detached from the node %q", pv.Spec.CSI.VolumeHandle, pod.Spec.NodeName))

		ginkgo.By(fmt.Sprintf("Waiting for pod name to be deleted for volume %s by metadata-syncer", pv.Spec.CSI.VolumeHandle))
		err = waitForPodNameLabelRemoval(pv.Spec.CSI.VolumeHandle, pod.Name, pod.Namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

})

func waitForPodNameLabelRemoval(volumeID string, podname string, namespace string) error {
	err := wait.Poll(poll, pollTimeout, func() (bool, error) {
		_, err := e2eVSphere.getLabelsForCNSVolume(volumeID, string(cnstypes.CnsKubernetesEntityTypePOD), podname, namespace)
		if err != nil {
			framework.Logf("pod name label is successfully removed")
			return true, err
		}
		framework.Logf("waiting for pod name label to be removed by metadata-syncer for volume: %q", volumeID)
		return false, nil
	})
	// unable to retrieve pod name label from vCenter
	if err != nil {
		return nil
	}
	return fmt.Errorf("pod name label is not removed from cns")
}
