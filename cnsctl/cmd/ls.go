/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

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
	"fmt"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/ov"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Orphan volume commands",
	Long:  "Orphan volume commands",
	Run: func(cmd *cobra.Command, args []string) {
		req := &ov.OrphanVolumeRequest{
			KubeConfigFile: cmd.Flag("kubeconfig").Value.String(),
			VcUser:         cmd.Flag("user").Value.String(),
			VcPwd:          cmd.Flag("password").Value.String(),
			VcHost:         cmd.Flag("host").Value.String(),
			Datacenter:     cmd.Flag("datacenter").Value.String(),
			Datastores:     strings.Split(cmd.Flag("datastores").Value.String(), ","),
		}
		res, err := ov.GetOrphanVolumes(req)
		if err != nil {
			fmt.Printf("Failed to get orphan volumes. Err: %+v\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 10, 1, ' ', tabwriter.TabIndent)
		fmt.Fprintf(w, "DATASTORE\tFCD_ID\tIS_ORPHAN\tPV_NAME\n")
		for _, fcdInfo := range res.Fcds {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fcdInfo.Datastore, fcdInfo.FcdId, strconv.FormatBool(fcdInfo.IsOrphan), fcdInfo.PvName)
		}
		w.Flush()
	},
}

func InitLs() {
	ovCmd.AddCommand(lsCmd)
}
