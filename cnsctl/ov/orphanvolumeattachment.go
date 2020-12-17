/*
Copyright 2020 The Kubernetes Authors.

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
package ov

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

type OrphanVolumeAttachmentReq struct {
	KubeClient *clientset.Clientset
}

type VolumeAttachmentInfo struct {
	VAName           string
	PVName           string
	AttachedNode     string
	AttachmentStatus bool
	IsOrphan         bool
}

type OrphanVolumeAttachmentResult struct {
	VAInfo        []VolumeAttachmentInfo
	TotalVA       int
	TotalOrphanVA int
}

func GetOrphanVolumeAttachments(ctx context.Context, req *OrphanVolumeAttachmentReq) (*OrphanVolumeAttachmentResult, error) {
	pvs, err := req.KubeClient.CoreV1().PersistentVolumes().List(ctx, v1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to list persistent volumes.")
		return nil, err
	}
	fmt.Printf("Found %d PVs in the Kubernetes cluster\n", len(pvs.Items))
	pvMap := make(map[string]struct{})
	for _, pv := range pvs.Items {
		pvMap[pv.Name] = struct{}{}
	}

	vaList, err := req.KubeClient.StorageV1().VolumeAttachments().List(ctx, v1.ListOptions{})
	if err != nil {
		fmt.Printf("Unable to list volume attachment CRs from the Kubernetes cluster.\n")
		return nil, err
	}
	res := &OrphanVolumeAttachmentResult{
		VAInfo:        make([]VolumeAttachmentInfo, 0, len(vaList.Items)),
		TotalVA:       0,
		TotalOrphanVA: 0,
	}
	for _, va := range vaList.Items {
		if va.Spec.Attacher == "csi.vsphere.vmware.com" {
			vaInfo := VolumeAttachmentInfo{
				VAName:           va.Name,
				PVName:           *va.Spec.Source.PersistentVolumeName,
				AttachedNode:     va.Spec.NodeName,
				AttachmentStatus: va.Status.Attached,
				IsOrphan:         false,
			}
			if _, ok := pvMap[*va.Spec.Source.PersistentVolumeName]; !ok {
				vaInfo.IsOrphan = true
				res.TotalOrphanVA++
			}
			res.VAInfo = append(res.VAInfo, vaInfo)
			res.TotalVA++
		}
	}
	return res, nil
}

func DeleteVolumeAttachments(ctx context.Context, kubeClient *clientset.Clientset, ovaRes *OrphanVolumeAttachmentResult) (int, error) {
	deleteCount := 0
	currOrphan := 1
	for _, vaInfo := range ovaRes.VAInfo {
		if vaInfo.IsOrphan {
			fmt.Printf("(%d/%d) Trying to delete VolumeAttachment: %s for PV %s\n", currOrphan, ovaRes.TotalOrphanVA, vaInfo.VAName, vaInfo.PVName)
			currOrphan++
			va, err := kubeClient.StorageV1().VolumeAttachments().Get(ctx, vaInfo.VAName, v1.GetOptions{})
			if err != nil {
				fmt.Printf("Unable to get the VolumeAttachment CR: %s. Err: %+v. Ignoring deletion of this VolumeAttachment..\n", vaInfo.VAName, err)
				continue
			}
			va.Finalizers = []string{}
			_, err = kubeClient.StorageV1().VolumeAttachments().Update(ctx, va, v1.UpdateOptions{})
			if err != nil {
				fmt.Printf("Unable to remove finalizers from VolumeAttachment CR: %s. Err: %+v. Ignoring deletion of this VolumeAttachment..\n", vaInfo.VAName, err)
				continue
			}
			var grace int64 = 0
			err = kubeClient.StorageV1().VolumeAttachments().Delete(ctx, vaInfo.VAName, v1.DeleteOptions{
				GracePeriodSeconds: &grace,
			})
			if apierrors.IsNotFound(err) {
				fmt.Printf("VolumeAttachment %s for PV %s is not found. Err: %+v. Likely that it is deleted.\n", vaInfo.VAName, vaInfo.PVName, err)
				deleteCount++
				continue
			}
			if err != nil {
				fmt.Printf("Unable to delete VolumeAttachment: %s for PV %s. Err: %+v. Ignoring deletion of this VolumeAttachment..\n", vaInfo.VAName, vaInfo.PVName, err)
				continue
			}
			fmt.Printf("Deleted VolumeAttachment: %s for PV %s\n", vaInfo.VAName, vaInfo.PVName)
			deleteCount++
		}
	}
	return deleteCount, nil
}
