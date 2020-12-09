package ov

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vslm"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	csitypes "sigs.k8s.io/vsphere-csi-driver/pkg/csi/types"
)

type OrphanVolumeRequest struct {
	KubeConfigFile string
	VcUser         string
	VcPwd          string
	VcHost         string
	Datacenter     string
	Datastores     []string
}

type FcdInfo struct {
	FcdId     string
	Datastore string
	PvName    string
	IsOrphan  bool
}

type OrphanVolumeResult struct {
	Fcds []FcdInfo
}

// GetOrphanVolumes provides the point in time orphan volumes that exists in FCD but used in Kubernetes cluster.
func GetOrphanVolumes(req *OrphanVolumeRequest) (*OrphanVolumeResult, error) {
	config, err := clientcmd.BuildConfigFromFlags("", req.KubeConfigFile)
	if err != nil {
		fmt.Printf("BuildConfigFromFlags failed %v\n", err)
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("KubeClient creation failed %v\n", err)
		return nil, err
	}
	ctx := context.Background()
	url := fmt.Sprintf("https://%s:%s@%s/sdk", req.VcUser, req.VcPwd, req.VcHost)
	u, err := soap.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, err
	}
	return GetOrphanVolumesWithClients(ctx, kubeClient, c, req.Datacenter, req.Datastores)
}

func GetOrphanVolumesWithClients(ctx context.Context, kubeClient kubernetes.Interface, vcClient *govmomi.Client, dc string, dss []string) (*OrphanVolumeResult, error) {
	res := &OrphanVolumeResult{
		Fcds: make([]FcdInfo, 0),
	}

	volumeHandleToPvMap := make(map[string]string)
	fmt.Printf("Listing all PVs in the Kubernetes cluster...")
	pvs, err := kubeClient.CoreV1().PersistentVolumes().List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	fmt.Printf("Found %d PVs in the Kubernetes cluster\n", len(pvs.Items))
	for _, pv := range pvs.Items {
		if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == csitypes.Name {
			volumeHandleToPvMap[pv.Spec.CSI.VolumeHandle] = pv.Name
		}
	}

	finder := find.NewFinder(vcClient.Client, false)
	dcObj, err := finder.Datacenter(ctx, dc)
	if err != nil {
		fmt.Printf("Unable to find datacenter: %s\n", dc)
		return nil, err
	}
	m := vslm.NewObjectManager(vcClient.Client)
	finder.SetDatacenter(dcObj)
	for _, ds := range dss {
		fmt.Printf("Listing FCDs under datastore: %s\n", ds)
		dsObj, err := finder.Datastore(ctx, ds)
		if err != nil {
			fmt.Printf("Unable to find datastore: %s\n", ds)
			return nil, err
		}
		fcds, err := m.List(ctx, dsObj)
		if err != nil {
			fmt.Printf("Failed to list FCDs in datastore: %s\n", ds)
			return nil, err
		}
		fmt.Printf("Found %d FCDs under datastore: %s\n", len(fcds), ds)
		for _, fcd := range fcds {
			if pv, ok := volumeHandleToPvMap[fcd.Id]; !ok {
				res.Fcds = append(res.Fcds, FcdInfo{
					FcdId:     fcd.Id,
					Datastore: ds,
					IsOrphan:  true,
				})
			} else {
				res.Fcds = append(res.Fcds, FcdInfo{
					FcdId:     fcd.Id,
					Datastore: ds,
					PvName:    pv,
					IsOrphan:  false,
				})
			}
		}
	}
	return res, nil
}
