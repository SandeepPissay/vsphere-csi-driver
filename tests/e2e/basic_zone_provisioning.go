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
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"time"
)

/*
	Test to verify provisioning volume with valid zone and region specified in Storage Class succeeds.
	Volume should be provisioned with Node Affinity rules for that zone/region.
	Pod should be scheduled on Node located within that zone/region.

	Steps
	1. Create a Storage Class with valid region and zone specified in “AllowedTopologies”
	2. Create a PVC using above SC
	3. Wait for PVC to be in bound phase
	4. Verify PV is created in specified zone and region
	5. Create a Pod attached to the above PV
	6. Verify Pod is scheduled on node located within the specified zone and region
	7. Delete Pod and wait for disk to be detached
	8. Delete PVC
	9. Delete Storage Class
*/

var _ = ginkgo.Describe("[csi-block-e2e-zone] Basic Topology Aware Provisioning", func() {
	f := framework.NewDefaultFramework("e2e-vsphere-basic-zone-provisioning")
	var (
		client            clientset.Interface
		namespace         string
		zoneValues        []string
		regionValues      []string
		pvZone            string
		pvRegion          string
		allowedTopologies []v1.TopologySelectorLabelRequirement
		nodeList          *v1.NodeList
		pod               *v1.Pod
		pvclaim           *v1.PersistentVolumeClaim
		storageclass      *storagev1.StorageClass
		err               error
		topologyMap       map[string][]string
	)
	ginkgo.BeforeEach(func() {
		client = f.ClientSet
		namespace = f.Namespace.Name
		bootstrap()
		nodeList = framework.GetReadySchedulableNodesOrDie(f.ClientSet)
		if !(len(nodeList.Items) > 0) {
			framework.Failf("Unable to find ready and schedulable Node")
		}
		topology := GetAndExpectStringEnvVar(envTopology)
		topologyMap = createTopologyMap(topology)
		regionValues, zoneValues = getValidTopology(topologyMap)
		allowedTopologies = []v1.TopologySelectorLabelRequirement{
			{
				Key:    zoneKey,
				Values: zoneValues,
			},
			{
				Key:    regionKey,
				Values: regionValues,
			},
		}
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("Performing cleanup")
		ginkgo.By("Deleting the pod and wait for disk to detach")
		err := framework.DeletePodWithWait(f, client, pod)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Deleting the PVC")
		err = framework.DeletePersistentVolumeClaim(client, pvclaim.Name, namespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Deleting the Storage Class")
		err = client.StorageV1().StorageClasses().Delete(storageclass.Name, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	// Provisioning with valid topology should be successful
	// Pod should be scheduled on Node located within that topology
	ginkgo.It("Verify provisioning with valid topology specified in Storage Class passes", func() {
		ginkgo.By("Creating Storage Class with allowedTopologies set and dynamically provisioning volume")
		storageclass, pvclaim, err = createPVCAndStorageClass(client, namespace, nil, nil, "", allowedTopologies)

		ginkgo.By("Expect claim to pass provisioning volume")
		err = framework.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, client, pvclaim.Namespace, pvclaim.Name, framework.Poll, time.Minute)
		gomega.Expect(err).NotTo(gomega.HaveOccurred(), fmt.Sprintf("Failed to provision volume with err: %v", err))

		ginkgo.By("Verify if volume is provisioned in specified zone and region")
		pv := getPvFromClaim(client, pvclaim.Namespace, pvclaim.Name)
		pvRegion, pvZone, err = verifyVolumeTopology(pv, zoneValues, regionValues)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Creating a pod")
		pod, err := framework.CreatePod(client, namespace, nil, []*v1.PersistentVolumeClaim{pvclaim}, false, "")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		ginkgo.By("Verify volume is attached to the node")
		isDiskAttached, err := e2eVSphere.isVolumeAttachedToNode(client, pv.Spec.CSI.VolumeHandle, pod.Spec.NodeName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(isDiskAttached).To(gomega.BeTrue(), fmt.Sprintf("Volume is not attached to the node"))

		ginkgo.By("Verify Pod is scheduled in on a node belonging to same topology as the PV it is attached to")
		err = verifyPodLocation(pod, nodeList, pvZone, pvRegion)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

})
