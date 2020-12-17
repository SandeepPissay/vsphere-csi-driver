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
	"github.com/spf13/viper"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/ov"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/virtualcenter/client"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var datastores, cfgFile string
var all, long bool

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List Orphan volumes",
	Long:  "List Orphan volumes",
	Run: func(cmd *cobra.Command, args []string) {
		validateOvFlags()
		validateLsFlags()

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
			LongListing:    cmd.Flag("long-list").Value.String() == "true",
		}
		res, err := ov.GetOrphanVolumes(ctx, req)
		if err != nil {
			fmt.Printf("Failed to get orphan volumes. Err: %+v\n", err)
			os.Exit(1)
		}
		var totalVols, totalOrphans, totalAttachedOrphans, totalDetachedOrphans int
		for _, fcdInfo := range res.Fcds {
			totalVols++
			if fcdInfo.IsOrphan {
				totalOrphans++
			}
			if fcdInfo.IsAttached {
				totalAttachedOrphans++
			} else {
				totalDetachedOrphans++
			}
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 10, 1, ' ', tabwriter.TabIndent)
		if totalVols > 0 {
			if cmd.Flag("all").Value.String() == "true" && cmd.Flag("long-list").Value.String() == "false" {
				if totalVols > 0 {
					fmt.Fprintf(w, "\nDATASTORE\tVOLUME_ID\tIS_ORPHAN\tPV_NAME\n")
					for _, fcdInfo := range res.Fcds {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fcdInfo.Datastore, fcdInfo.FcdId, strconv.FormatBool(fcdInfo.IsOrphan), fcdInfo.PvName)
					}
				} else {
					fmt.Printf("Volumes not found.\n")
				}
			} else if cmd.Flag("all").Value.String() == "true" && cmd.Flag("long-list").Value.String() == "true" {
				if totalVols > 0 {
					fmt.Fprintf(w, "\nDATASTORE\tVOLUME_ID\tIS_ATTACHED\tATTACHED_VM\tCREATE_TIME\tCAPACITY_MB\tIS_ORPHAN\tPV_NAME\n")
					for _, fcdInfo := range res.Fcds {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n", fcdInfo.Datastore, fcdInfo.FcdId,
							strconv.FormatBool(fcdInfo.IsAttached), fcdInfo.AttachedVmName,
							fcdInfo.CreateTime.String(), fcdInfo.CapacityInMB, strconv.FormatBool(fcdInfo.IsOrphan), fcdInfo.PvName)
					}
				} else {
					fmt.Printf("Volumes not found.\n")
				}
			} else if cmd.Flag("all").Value.String() == "false" && cmd.Flag("long-list").Value.String() == "true" {
				if totalOrphans > 0 {
					fmt.Fprintf(w, "\nDATASTORE\tORPHAN_VOLUME\tIS_ATTACHED\tATTACHED_VM\tCREATE_TIME\tCAPACITY_MB\n")
					for _, fcdInfo := range res.Fcds {
						if fcdInfo.IsOrphan {
							fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n", fcdInfo.Datastore, fcdInfo.FcdId,
								strconv.FormatBool(fcdInfo.IsAttached), fcdInfo.AttachedVmName,
								fcdInfo.CreateTime.String(), fcdInfo.CapacityInMB)
						}
					}
				} else {
					fmt.Printf("Orphan volumes not found.\n")
				}
			} else {
				if totalOrphans > 0 {
					fmt.Fprintf(w, "\nDATASTORE\tORPHAN_VOLUME\n")
					for _, fcdInfo := range res.Fcds {
						if fcdInfo.IsOrphan {
							fmt.Fprintf(w, "%s\t%s\n", fcdInfo.Datastore, fcdInfo.FcdId)
						}
					}
				} else {
					fmt.Printf("Orphan volumes not found.\n")
				}
			}
		}
		w.Flush()
		fmt.Printf("\n----------------------- Summary ------------------------------\n")
		fmt.Printf("Total FCDs on the datastore(s): %d\n", totalVols)
		fmt.Printf("Total orphan volumes on the datastore(s): %d\n", totalOrphans)
		fmt.Printf("Total attached orphan volumes on the datastore(s): %d\n", totalAttachedOrphans)
		fmt.Printf("Total detached orphan volumes on the datastore(s): %d\n", totalDetachedOrphans)
	},
}

func InitLs() {
	lsCmd.PersistentFlags().StringVarP(&datastores, "datastores", "d", viper.GetString("datastores"), "comma-separated datastore names (alternatively use CNSCTL_DATASTORES env variable)")
	lsCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file (alternatively use CNSCTL_KUBECONFIG env variable)")
	lsCmd.PersistentFlags().BoolVarP(&all, "all", "a", false, "Show orphan and used volumes")
	lsCmd.PersistentFlags().BoolVarP(&long, "long-list", "l", false, "Show additional details of the volumes")
	ovCmd.AddCommand(lsCmd)
}

func validateLsFlags() {
	if datastores == "" {
		fmt.Printf("error: datastores flag or CNSCTL_DATASTORES env variable must be set for 'ls' sub-command\n")
		os.Exit(1)
	}
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set for 'ls' sub-command")
		os.Exit(1)
	}
}
