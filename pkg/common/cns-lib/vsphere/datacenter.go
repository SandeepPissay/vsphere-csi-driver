// Copyright 2018 VMware, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vsphere

import (
	"context"
	"fmt"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/tags"
	"net/url"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/klog"
)

// DatastoreInfoProperty refers to the property name info for the Datastore
const DatastoreInfoProperty = "info"

// Datacenter holds virtual center information along with the Datacenter.
type Datacenter struct {
	// Datacenter represents the govmomi Datacenter.
	*object.Datacenter
	// VirtualCenterHost represents the virtual center host address.
	VirtualCenterHost string
}

func (dc *Datacenter) String() string {
	return fmt.Sprintf("Datacenter [Datacenter: %v, VirtualCenterHost: %v]",
		dc.Datacenter, dc.VirtualCenterHost)
}

// GetDatastoreByURL returns the *Datastore instance given its URL.
func (dc *Datacenter) GetDatastoreByURL(ctx context.Context, datastoreURL string) (*Datastore, error) {
	finder := find.NewFinder(dc.Datacenter.Client(), false)
	finder.SetDatacenter(dc.Datacenter)
	datastores, err := finder.DatastoreList(ctx, "*")
	if err != nil {
		klog.Errorf("Failed to get all the datastores. err: %+v", err)
		return nil, err
	}
	var dsList []types.ManagedObjectReference
	for _, ds := range datastores {
		dsList = append(dsList, ds.Reference())
	}

	var dsMoList []mo.Datastore
	pc := property.DefaultCollector(dc.Client())
	properties := []string{DatastoreInfoProperty}
	err = pc.Retrieve(ctx, dsList, properties, &dsMoList)
	if err != nil {
		klog.Errorf("Failed to get Datastore managed objects from datastore objects."+
			" dsObjList: %+v, properties: %+v, err: %v", dsList, properties, err)
		return nil, err
	}
	for _, dsMo := range dsMoList {
		if dsMo.Info.GetDatastoreInfo().Url == datastoreURL {
			return &Datastore{object.NewDatastore(dc.Client(), dsMo.Reference()),
				dc}, nil
		}
	}
	err = fmt.Errorf("Couldn't find Datastore given URL %q", datastoreURL)
	klog.Error(err)
	return nil, err
}

// GetVirtualMachineByUUID returns the VirtualMachine instance given its UUID in a datacenter.
// If instanceUUID is set to true, then UUID is an instance UUID.
//  - In this case, this function searches for virtual machines whose instance UUID matches the given uuid.
// If instanceUUID is set to false, then UUID is BIOS UUID.
//  - In this case, this function searches for virtual machines whose BIOS UUID matches the given uuid.
func (dc *Datacenter) GetVirtualMachineByUUID(ctx context.Context, uuid string, instanceUUID bool) (*VirtualMachine, error) {
	uuid = strings.ToLower(strings.TrimSpace(uuid))
	searchIndex := object.NewSearchIndex(dc.Datacenter.Client())
	svm, err := searchIndex.FindByUuid(ctx, dc.Datacenter, uuid, true, &instanceUUID)
	if err != nil {
		klog.Errorf("Failed to find VM given uuid %s with err: %v", uuid, err)
		return nil, err
	} else if svm == nil {
		klog.Errorf("Couldn't find VM given uuid %s", uuid)
		return nil, ErrVMNotFound
	}
	vm := &VirtualMachine{
		VirtualCenterHost: dc.VirtualCenterHost,
		UUID:              uuid,
		VirtualMachine:    object.NewVirtualMachine(dc.Datacenter.Client(), svm.Reference()),
		Datacenter:        dc,
	}
	return vm, nil
}

// asyncGetAllDatacenters returns *Datacenter instances over the given
// channel. If an error occurs, it will be returned via the given error channel.
// If the given context is canceled, the processing will be stopped as soon as
// possible, and the channels will be closed before returning.
func asyncGetAllDatacenters(ctx context.Context, dcsChan chan<- *Datacenter, errChan chan<- error) {
	defer close(dcsChan)
	defer close(errChan)

	for _, vc := range GetVirtualCenterManager().GetAllVirtualCenters() {
		// If the context was canceled, we stop looking for more Datacenters.
		select {
		case <-ctx.Done():
			err := ctx.Err()
			klog.V(2).Infof("Context was done, returning with err: %v", err)
			errChan <- err
			return
		default:
		}

		if err := vc.Connect(ctx); err != nil {
			klog.Errorf("Failed connecting to VC %q with err: %v", vc.Config.Host, err)
			errChan <- err
			return
		}

		dcs, err := vc.GetDatacenters(ctx)
		if err != nil {
			klog.Errorf("Failed to fetch datacenters for vc %v with err: %v", vc.Config.Host, err)
			errChan <- err
			return
		}

		for _, dc := range dcs {
			// If the context was canceled, we don't return more Datacenters.
			select {
			case <-ctx.Done():
				err := ctx.Err()
				klog.V(2).Infof("Context was done, returning with err: %v", err)
				errChan <- err
				return
			default:
				klog.V(2).Infof("Publishing datacenter %v", dc)
				dcsChan <- dc
			}
		}
	}
}

// AsyncGetAllDatacenters fetches all Datacenters asynchronously. The
// *Datacenter chan returns a *Datacenter on discovering one. The
// error chan returns a single error if one occurs. Both channels are closed
// when nothing more is to be sent.
//
// The buffer size for the *Datacenter chan can be specified via the
// buffSize parameter. For example, buffSize could be 1, in which case, the
// sender will buffer at most 1 *Datacenter instance (and possibly close
// the channel and terminate, if that was the only instance found).
//
// Note that a context.Canceled error would be returned if the context was
// canceled at some point during the execution of this function.
func AsyncGetAllDatacenters(ctx context.Context, buffSize int) (<-chan *Datacenter, <-chan error) {
	dcsChan := make(chan *Datacenter, buffSize)
	errChan := make(chan error, 1)
	go asyncGetAllDatacenters(ctx, dcsChan, errChan)
	return dcsChan, errChan
}

// GetVMMoList gets the VM Managed Objects with the given properties from the VM object
func (dc *Datacenter) GetVMMoList(ctx context.Context, vmObjList []*VirtualMachine, properties []string) ([]mo.VirtualMachine, error) {
	var vmMoList []mo.VirtualMachine
	var vmRefs []types.ManagedObjectReference
	if len(vmObjList) < 1 {
		msg := fmt.Sprintf("VirtualMachine Object list is empty")
		klog.Errorf(msg+": %v", vmObjList)
		return nil, fmt.Errorf(msg)
	}

	for _, vmObj := range vmObjList {
		vmRefs = append(vmRefs, vmObj.Reference())
	}
	pc := property.DefaultCollector(dc.Client())
	err := pc.Retrieve(ctx, vmRefs, properties, &vmMoList)
	if err != nil {
		klog.Errorf("Failed to get VM managed objects from VM objects. vmObjList: %+v, properties: %+v, err: %v", vmObjList, properties, err)
		return nil, err
	}
	return vmMoList, nil
}

// GetAllDatastores gets the datastore URL to DatastoreInfo map for all the datastores in
// the datacenter.
func (dc *Datacenter) GetAllDatastores(ctx context.Context) (map[string]*DatastoreInfo, error) {
	finder := find.NewFinder(dc.Client(), false)
	finder.SetDatacenter(dc.Datacenter)
	datastores, err := finder.DatastoreList(ctx, "*")
	if err != nil {
		klog.Errorf("Failed to get all the datastores in the Datacenter %s with error: %v", dc.Datacenter.String(), err)
		return nil, err
	}
	var dsList []types.ManagedObjectReference
	for _, ds := range datastores {
		dsList = append(dsList, ds.Reference())
	}
	var dsMoList []mo.Datastore
	pc := property.DefaultCollector(dc.Client())
	properties := []string{"info"}
	err = pc.Retrieve(ctx, dsList, properties, &dsMoList)
	if err != nil {
		klog.Errorf("Failed to get datastore managed objects from datastore objects %v with properties %v: %v", dsList, properties, err)
		return nil, err
	}
	dsURLInfoMap := make(map[string]*DatastoreInfo)
	for _, dsMo := range dsMoList {
		dsURLInfoMap[dsMo.Info.GetDatastoreInfo().Url] = &DatastoreInfo{
			&Datastore{object.NewDatastore(dc.Client(), dsMo.Reference()),
				dc},
			dsMo.Info.GetDatastoreInfo()}
	}
	return dsURLInfoMap, nil
}

// IsMoRefInZoneRegion checks specified moRef belongs to specified zone and region
// This function returns true if moRef belongs to zone/region, else returns false.
func (dc *Datacenter) IsMoRefInZoneRegion(ctx context.Context, moRef types.ManagedObjectReference, zoneKey string, regionKey string, zoneValue string, regionValue string) (bool, error) {
	klog.V(4).Infof("IsMoRefInZoneRegion: called with moRef: %v, zonekey: %s, regionkey: %s, zonevalue: %s, regionvalue: %s", moRef, zoneKey, regionKey, zoneValue, regionValue)
	var err error
	result := make(map[string]string, 0)
	tagManager := tags.NewManager(rest.NewClient(dc.Client()))
	virtualCenter, err := GetVirtualCenterManager().GetVirtualCenter(dc.VirtualCenterHost)
	if err != nil {
		klog.Errorf("Failed to get virtualCenter. Error: %v", err)
		return false, err
	}
	user := url.UserPassword(virtualCenter.Config.Username, virtualCenter.Config.Password)
	if err := tagManager.Login(ctx, user); err != nil {
		klog.Errorf("Failed to login for tagManager. err %v", moRef, err)
		return false, err
	}
	defer tagManager.Logout(ctx)

	var objects []mo.ManagedEntity
	pc := dc.Client().ServiceContent.PropertyCollector
	// example result: ["Folder", "Datacenter", "Cluster", "Host"]
	objects, err = mo.Ancestors(ctx, dc.Client(), pc, moRef)
	if err != nil {
		klog.Errorf("Ancestors failed for %s with err %v", moRef, err)
		return false, err
	}
	// search the hierarchy, example order: ["Host", "Cluster", "Datacenter", "Folder"]
	for i := range objects {
		obj := objects[len(objects)-1-i]
		klog.V(4).Infof("Name: %s, Type: %s", obj.Self.Value, obj.Self.Type)
		tags, err := tagManager.ListAttachedTags(ctx, obj)
		if err != nil {
			klog.Errorf("Cannot list attached tags. Err: %v", err)
			return false, err
		}
		klog.V(4).Infof("Object [%v] has attached Tags [%v]", tags, obj)
		for _, value := range tags {
			tag, err := tagManager.GetTag(ctx, value)
			if err != nil {
				klog.Errorf("Failed to get tag:%s, error:%v", value, err)
				return false, err
			}
			klog.V(4).Infof("Found tag: %s for object %v", tag.Name, obj)
			category, err := tagManager.GetCategory(ctx, tag.CategoryID)
			if err != nil {
				klog.Errorf("Failed to get category for tag: %s, error: %v", tag.Name, tag)
				return false, err
			}
			klog.V(4).Infof("Found category: %s for object %v with tag: %s", category.Name, obj, tag.Name)
			found := func() {
				klog.V(4).Infof("Found requested category: %s and tag: %s attached to %s", category.Name, tag.Name, moRef)
			}

			switch {
			case category.Name == zoneKey:
				result["zone"] = tag.Name
				found()
			case category.Name == regionKey:
				result["region"] = tag.Name
				found()
			}

			if regionValue == "" && zoneValue != "" && result["zone"] == zoneValue {
				// region is not specified, if zone matches with look up zone value, return true
				klog.V(4).Infof("MoRef [%v] belongs to zone [%s]", moRef, zoneValue)
				return true, nil
			}
			if zoneValue == "" && regionValue != "" && result["region"] == regionValue {
				// zone is not specified, if region matches with look up region value, return true
				klog.V(4).Infof("MoRef [%v] belongs to region [%s]", moRef, regionValue)
				return true, nil
			}
			if result["zone"] != "" && result["region"] != "" {
				if result["region"] == regionValue && result["zone"] == zoneValue {
					klog.V(4).Infof("MoRef [%v] belongs to zone [%s] and region [%s]", moRef, zoneValue, regionValue)
					return true, nil
				}
			}
		}
	}
	return false, nil
}
