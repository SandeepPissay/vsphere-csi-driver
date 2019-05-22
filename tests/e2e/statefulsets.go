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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	manifestPath     = "tests/e2e/testing-manifests/statefulset/nginx"
	mountPath        = "/usr/share/nginx/html"
	storageclassname = "nginx-sc"
)

/*
	Test performs following operations

	Steps
	1. Create a storage class.
	2. Create nginx service.
	3. Create nginx statefulsets with 3 replicas.
	4. Wait until all Pods are ready and PVCs are bounded with PV.
	5. Scale down statefulsets to 2 replicas.
	6. Scale up statefulsets to 3 replicas.
	7. Scale down statefulsets to 0 replicas and delete all pods.
	8. Delete all PVCs from the tests namespace.
	9. Delete the storage class.
*/

var _ = Describe("[csi-block-e2e] statefulset", func() {
	f := framework.NewDefaultFramework("e2e-vsphere-statefulset")
	var (
		namespace string
		client    clientset.Interface
	)
	BeforeEach(func() {
		namespace = f.Namespace.Name
		client = f.ClientSet
		bootstrap()
		sc, err := client.StorageV1().StorageClasses().Get(storageclassname, metav1.GetOptions{})
		if err == nil && sc != nil {
			Expect(client.StorageV1().StorageClasses().Delete(sc.Name, nil)).NotTo(HaveOccurred())
		}
	})
	AfterEach(func() {
		framework.Logf("Deleting all statefulset in namespace: %v", namespace)
		framework.DeleteAllStatefulSets(client, namespace)
	})

	It("vsphere statefulset testing", func() {
		By("Creating StorageClass for Statefulset")
		scSpec := getVSphereStorageClassSpec(storageclassname, nil)
		sc, err := client.StorageV1().StorageClasses().Create(scSpec)
		Expect(err).NotTo(HaveOccurred())
		defer client.StorageV1().StorageClasses().Delete(sc.Name, nil)

		By("Creating statefulset")
		statefulsetTester := framework.NewStatefulSetTester(client)
		statefulset := statefulsetTester.CreateStatefulSet(manifestPath, namespace)
		replicas := *(statefulset.Spec.Replicas)
		// Waiting for pods status to be Ready
		statefulsetTester.WaitForStatusReadyReplicas(statefulset, replicas)
		Expect(statefulsetTester.CheckMount(statefulset, mountPath)).NotTo(HaveOccurred())
		ssPodsBeforeScaleDown := statefulsetTester.GetPodList(statefulset)
		Expect(ssPodsBeforeScaleDown.Items).NotTo(BeEmpty(), fmt.Sprintf("Unable to get list of Pods from the Statefulset: %v", statefulset.Name))
		Expect(len(ssPodsBeforeScaleDown.Items) == int(replicas)).To(BeTrue(), "Number of Pods in the statefulset should match with number of replicas")

		// Get the list of Volumes attached to Pods before scale down
		var volumesBeforeScaleDown []string
		for _, sspod := range ssPodsBeforeScaleDown.Items {
			_, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			for _, volumespec := range sspod.Spec.Volumes {
				if volumespec.PersistentVolumeClaim != nil {
					pv := getPvFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
					volumeId := pv.Spec.CSI.VolumeHandle
					volumesBeforeScaleDown = append(volumesBeforeScaleDown, volumeId)
					// Verify the attached volume match the one in CNS cache
					err := verifyVolumeMetadataInCNS(&e2eVSphere, volumeId, volumespec.PersistentVolumeClaim.ClaimName, pv.ObjectMeta.Name, sspod.Name)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}

		By(fmt.Sprintf("Scaling down statefulsets to number of Replica: %v", replicas-1))
		_, scaledownErr := statefulsetTester.Scale(statefulset, replicas-1)
		Expect(scaledownErr).NotTo(HaveOccurred())
		statefulsetTester.WaitForStatusReadyReplicas(statefulset, replicas-1)
		ssPodsAfterScaleDown := statefulsetTester.GetPodList(statefulset)
		Expect(ssPodsAfterScaleDown.Items).NotTo(BeEmpty(), fmt.Sprintf("Unable to get list of Pods from the Statefulset: %v", statefulset.Name))
		Expect(len(ssPodsAfterScaleDown.Items) == int(replicas-1)).To(BeTrue(), "Number of Pods in the statefulset should match with number of replicas")

		// After scale down, verify vsphere volumes are detached from deleted pods
		By("Verify Volumes are detached from Nodes after Statefulsets is scaled down")
		for _, sspod := range ssPodsBeforeScaleDown.Items {
			_, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			if err != nil {
				Expect(apierrs.IsNotFound(err), BeTrue())
				for _, volumespec := range sspod.Spec.Volumes {
					if volumespec.PersistentVolumeClaim != nil {
						pv := getPvFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
						framework.Logf("FCD ID : %s", pv.Spec.CSI.VolumeHandle)
						vmuuid := getNodeUUID(client, sspod.Spec.NodeName)
						framework.Logf("vmuuid ID : %s for Node: %s", vmuuid, sspod.Spec.NodeName)
						isDiskDetached, err := isCNSDiskDetached(&e2eVSphere, vmuuid, pv.Spec.CSI.VolumeHandle)
						Expect(err).NotTo(HaveOccurred())
						Expect(isDiskDetached).To(BeTrue(), fmt.Sprintf("Volume is not detached from the node"))
					}
				}
			}
		}

		// After scale down, verify the attached volumes match those in CNS Cache
		for _, sspod := range ssPodsAfterScaleDown.Items {
			_, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			for _, volumespec := range sspod.Spec.Volumes {
				if volumespec.PersistentVolumeClaim != nil {
					pv := getPvFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
					err := verifyVolumeMetadataInCNS(&e2eVSphere, pv.Spec.CSI.VolumeHandle, volumespec.PersistentVolumeClaim.ClaimName, pv.ObjectMeta.Name, sspod.Name)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}

		By(fmt.Sprintf("Scaling up statefulsets to number of Replica: %v", replicas))
		_, scaleupErr := statefulsetTester.Scale(statefulset, replicas)
		Expect(scaleupErr).NotTo(HaveOccurred())
		statefulsetTester.WaitForStatusReplicas(statefulset, replicas)
		statefulsetTester.WaitForStatusReadyReplicas(statefulset, replicas)

		ssPodsAfterScaleUp := statefulsetTester.GetPodList(statefulset)
		Expect(ssPodsAfterScaleUp.Items).NotTo(BeEmpty(), fmt.Sprintf("Unable to get list of Pods from the Statefulset: %v", statefulset.Name))
		Expect(len(ssPodsAfterScaleUp.Items) == int(replicas)).To(BeTrue(), "Number of Pods in the statefulset should match with number of replicas")

		// After scale up, verify all vsphere volumes are attached to node VMs.
		By("Verify all volumes are attached to Nodes after Statefulsets is scaled up")
		for _, sspod := range ssPodsAfterScaleUp.Items {
			err := framework.WaitForPodsReady(client, statefulset.Namespace, sspod.Name, 0)
			Expect(err).NotTo(HaveOccurred())
			pod, err := client.CoreV1().Pods(namespace).Get(sspod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			for _, volumespec := range pod.Spec.Volumes {
				if volumespec.PersistentVolumeClaim != nil {
					pv := getPvFromClaim(client, statefulset.Namespace, volumespec.PersistentVolumeClaim.ClaimName)
					volumeId := pv.Spec.CSI.VolumeHandle
					By("Verify scale up operation should not introduced new volume")
					Expect(contains(volumesBeforeScaleDown, volumeId)).To(BeTrue())
					vmuuid := getNodeUUID(client, pod.Spec.NodeName)
					framework.Logf("vmuuid ID : %s for Node: %s", vmuuid, sspod.Spec.NodeName)
					isAttached, err := e2eVSphere.verifyCNSVolumeIsAttached(vmuuid, volumeId)
					Expect(err).NotTo(HaveOccurred(), "Disk is not attached to the node")
					Expect(isAttached).To(BeTrue(), fmt.Sprintf("Disk is not attached"))
					By("After scale up, verify the attached volumes match those in CNS Cache")
					err = verifyVolumeMetadataInCNS(&e2eVSphere, pv.Spec.CSI.VolumeHandle, volumespec.PersistentVolumeClaim.ClaimName, pv.ObjectMeta.Name, sspod.Name)
					Expect(err).NotTo(HaveOccurred())
				}
			}
		}
	})
})

// check whether the slice contains an element
func contains(volumes []string, volumeId string) bool {
	for _, volumeID := range volumes {
		if volumeID == volumeId {
			return true
		}
	}
	return false
}
