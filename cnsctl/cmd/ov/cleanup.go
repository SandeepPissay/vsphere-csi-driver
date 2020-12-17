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
	"github.com/spf13/viper"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/ov"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/client"
	"strings"

	"github.com/spf13/cobra"
)

// cleanupCmd represents the cleanup command
var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Identifies orphan volumes and deletes them",
	Long:  "Identifies orphan volumes and deletes them",
	Run: func(cmd *cobra.Command, args []string) {
		validateOvFlags()
		validateCleanupFlags()

		if len(args) != 0 {
			fmt.Printf("error: no arguments allowed for cleanup\n")
			os.Exit(1)
		}
		ctx := context.Background()
		vcClient, err := client.GetClient(ctx, cmd.Flag("user").Value.String(), cmd.Flag("password").Value.String(), cmd.Flag("host").Value.String())
		if err != nil {
			fmt.Printf("error: failed to get vcClient: %+v\n", err)
			os.Exit(1)
		}

		req := &ov.OrphanVolumeRequest{
			KubeConfigFile: cmd.Flag("kubeconfig").Value.String(),
			VcClient:       vcClient,
			Datacenter:     cmd.Flag("datacenter").Value.String(),
			Datastores:     strings.Split(cmd.Flag("datastores").Value.String(), ","),
		}

		res, err := ov.GetOrphanVolumes(ctx, req)
		if err != nil {
			fmt.Printf("Failed to get orphan volumes. Err: %+v\n", err)
			os.Exit(1)
		}
		totalOrphans := 0
		for _, fcdInfo := range res.Fcds {
			if fcdInfo.IsOrphan == true {
				totalOrphans++
			}
		}
		totalCleanedOrphans := 0
		currOrphan := 1
		for _, fcdInfo := range res.Fcds {
			if fcdInfo.IsOrphan == true {
				fmt.Printf("(%d/%d) Cleaning orphan volume: %+v\n", currOrphan, totalOrphans, fcdInfo)
				currOrphan++
				deleteCount := deleteVolume(ctx, vcClient, []string{fcdInfo.FcdId}, fcdInfo.Datastore, cmd.Flag("datacenter").Value.String(), cmd.Flag("force").Value.String())
				totalCleanedOrphans += deleteCount
			}
		}
		fmt.Printf("\n----------------------- Summary ------------------------------\n")
		if totalCleanedOrphans > 0 {
			fmt.Printf("Cleaned up %d orphan volumes.\n", totalOrphans)
		} else {
			fmt.Printf("Orphan volumes were not found or they were not cleaned up.\n")
		}
	},
}

func InitCleanup() {
	cleanupCmd.PersistentFlags().StringVarP(&datastores, "datastores", "d", viper.GetString("datastores"), "comma-separated datastore names (alternatively use CNSCTL_DATASTORES env variable)")
	cleanupCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file (alternatively use CNSCTL_KUBECONFIG env variable)")
	cleanupCmd.PersistentFlags().BoolVarP(&forceDelete, "force", "f", false, "force delete the volumes")
	ovCmd.AddCommand(cleanupCmd)
}

func validateCleanupFlags() {
	if datastores == "" {
		fmt.Printf("error: datastores flag or CNSCTL_DATASTORES env variable must be set for 'cleanup' sub-command\n")
		os.Exit(1)
	}
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set for 'ls' sub-command")
		os.Exit(1)
	}
}
