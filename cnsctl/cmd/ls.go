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
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var datastores, cfgFile string

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "Orphan volume commands",
	Long:  "Orphan volume commands",
	Run: func(cmd *cobra.Command, args []string) {
		validateOvFlags()
		validateLsConfig()

		req := &ov.OrphanVolumeRequest{
			KubeConfigFile: cmd.Flag("kubeconfig").Value.String(),
			VcUser:         cmd.Flag("user").Value.String(),
			VcPwd:          cmd.Flag("password").Value.String(),
			VcHost:         cmd.Flag("host").Value.String(),
			Datacenter:     cmd.Flag("datacenter").Value.String(),
			Datastores:     strings.Split(cmd.Flag("datastores").Value.String(), ","),
		}
		ctx := context.Background()
		res, err := ov.GetOrphanVolumes(ctx, req)
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
	lsCmd.PersistentFlags().StringVarP(&datastores, "datastores", "d", viper.GetString("datastores"), "Comma-separated datastore names")
	lsCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file")
	ovCmd.AddCommand(lsCmd)
}

func validateLsConfig() {
	if datastores == "" {
		fmt.Printf("error: datastores flag or CNSCTL_DATASTORES env variable must be set for 'ls' sub-command\n")
		os.Exit(1)
	}
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set for 'ls' sub-command")
		os.Exit(1)
	}
}