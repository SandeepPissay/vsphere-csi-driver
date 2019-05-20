package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"k8s.io/kubernetes/test/e2e/framework"
	cnsmethods "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/methods"
	cnstypes "sigs.k8s.io/vsphere-csi-driver/pkg/common/cns-lib/vmomi/types"
	"strings"
)

type VSphere struct {
	Config    *e2eTestConfig
	Client    *govmomi.Client
	CnsClient *cnsClient
}

const (
	ProviderPrefix = "vsphere://"
)

// queryCNSVolumeWithResult Call CnsQueryVolume and returns CnsQueryResult to client
func (vs *VSphere) queryCNSVolumeWithResult(fcdID string) (*cnstypes.CnsQueryResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Connect to VC
	connect(ctx, vs)
	var volumeIds []cnstypes.CnsVolumeId
	volumeIds = append(volumeIds, cnstypes.CnsVolumeId{
		Id: fcdID,
	})
	queryFilter := cnstypes.CnsQueryFilter{
		VolumeIds: volumeIds,
		Cursor: &cnstypes.CnsCursor{
			Offset: 0,
			Limit:  100,
		},
	}
	req := cnstypes.CnsQueryVolume{
		This:   cnsVolumeManagerInstance,
		Filter: queryFilter,
	}

	err := connectCns(ctx, vs)
	if err != nil {
		return nil, err
	}
	res, err := cnsmethods.CnsQueryVolume(ctx, vs.CnsClient.Client, &req)
	if err != nil {
		return nil, err
	}
	return &res.Returnval, nil
}

// getAllDatacenters returns all the DataCenter Objects
func (vs *VSphere) getAllDatacenters(ctx context.Context) ([]*object.Datacenter, error) {
	connect(ctx, vs)
	finder := find.NewFinder(vs.Client.Client, false)
	return finder.DatacenterList(ctx, "*")
}

// getVMByUUID gets the VM object Reference from the given vmUUID
func (vs *VSphere) getVMByUUID(ctx context.Context, vmUUID string) (object.Reference, error) {
	connect(ctx, vs)
	dcList, err := vs.getAllDatacenters(ctx)
	Expect(err).NotTo(HaveOccurred())
	for _, dc := range dcList {
		datacenter := object.NewDatacenter(vs.Client.Client, dc.Reference())
		s := object.NewSearchIndex(vs.Client.Client)
		vmUUID = strings.ToLower(strings.TrimSpace(vmUUID))
		vmMoRef, err := s.FindByUuid(ctx, datacenter, vmUUID, true, nil)
		if err != nil {
			continue
		}
		return vmMoRef, nil
	}
	return nil, fmt.Errorf("Node VM with UUID:%s is not found", vmUUID)
}

// verifyCNSVolumeIsAttached verifies CNS volume is attached to the node specified by its VM UUID
func (vs *VSphere) verifyCNSVolumeIsAttached(vmUUID string, volumeID string) (bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	vmRef, err := vs.getVMByUUID(ctx, vmUUID)
	Expect(err).NotTo(HaveOccurred())
	vm := object.NewVirtualMachine(vs.Client.Client, vmRef.Reference())
	device, err := getVirtualDeviceByDiskID(ctx, vm, volumeID)
	if err != nil {
		framework.Logf("failed to determine whether disk %q is still attached on node with UUID %q", volumeID, vmUUID)
		return false, err
	}
	if device == nil {
		return false, nil
	}
	framework.Logf("Found the disk %q is attached on node with UUID %q", volumeID, vmUUID)
	return true, nil
}
