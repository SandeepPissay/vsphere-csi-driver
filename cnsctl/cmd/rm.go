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
package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/vmware/govmomi"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/client"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/volume"
)

var datastore string
var forceDelete bool

// rmCmd represents the rm command
var rmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove specified volumes",
	Long:  "Remove specified volumes",
	Run: func(cmd *cobra.Command, args []string) {
		validateOvFlags()
		validateRmFlags()
		if len(args) == 0 {
			fmt.Printf("error: no volumes specified to be deleted.\n")
			os.Exit(1)
		}
		ctx := context.Background()
		vcClient, err := client.GetClient(ctx, cmd.Flag("user").Value.String(), cmd.Flag("password").Value.String(), cmd.Flag("host").Value.String())
		if err != nil {
			fmt.Printf("error: failed to get vcClient: %+v\n", err)
			os.Exit(1)
		}
		deleteVolume(ctx, vcClient, args, cmd.Flag("datastore").Value.String(), cmd.Flag("datacenter").Value.String(), cmd.Flag("force").Value.String())
	},
}

func deleteVolume(ctx context.Context, vcClient *govmomi.Client, fcds []string, ds string, dc string, forceDelete string) {
	for _, vol := range fcds {
		fmt.Printf("Trying to delete volume: %s\n", vol)
		deleteFcdRequest := &volume.DeleteFcdRequest{
			Client:     vcClient,
			FcdId:      vol,
			Datastore:  ds,
			Datacenter: dc,
		}
		err := volume.DeleteFcd(ctx, deleteFcdRequest, forceDelete)
		if err != nil {
			fmt.Printf("error: failed to delete FCD: %s. err: %+v.\nContinuing to delete other volumes if any...\n", vol, err)
		}
	}
}

func InitRm() {
	rmCmd.PersistentFlags().StringVarP(&datastore, "datastore", "d", "", "Datastore name")
	rmCmd.PersistentFlags().BoolVarP(&forceDelete, "force", "f", false, "Force delete the volume")
	ovCmd.AddCommand(rmCmd)
}

func validateRmFlags() {
	if datastore == "" {
		fmt.Printf("error: datastore flag must be set for 'rm' sub-command\n")
		os.Exit(1)
	}
}