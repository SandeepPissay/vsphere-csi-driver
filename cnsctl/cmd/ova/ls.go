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
package ova

import (
	"context"
	"fmt"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"sigs.k8s.io/vsphere-csi-driver/cnsctl/ov"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var cfgFile string
var all bool

// lsCmd represents the ls command
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List orphan VolumeAttachment CRs in Kubernetes",
	Long:  "List orphan VolumeAttachment CRs in Kubernetes",
	Run: func(cmd *cobra.Command, args []string) {
		validateLsFlags()
		kubeConfigFile := cmd.Flag("kubeconfig").Value.String()
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigFile)
		if err != nil {
			fmt.Printf("BuildConfigFromFlags failed %v\n", err)
			os.Exit(1)
		}
		kubeClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Printf("KubeClient creation failed %v\n", err)
			os.Exit(1)
		}
		ctx := context.Background()
		ovaReq := &ov.OrphanVolumeAttachmentReq{
			KubeClient: kubeClient,
		}
		res, err := ov.GetOrphanVolumeAttachments(ctx, ovaReq)
		if err != nil {
			fmt.Printf("Unable to find orphan volume attachments. Err: %s\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 10, 1, ' ', tabwriter.TabIndent)
		if cmd.Flag("all").Value.String() == "true" {
			if res.TotalVA > 0 {
				fmt.Fprintf(w, "\nVOLUME_ATTACHMENT_NAME\tPV_NAME\tIS_ORPHAN\tATTACH_NODE\tIS_ATTACHED\n")
				for _, vaInfo := range res.VAInfo {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", vaInfo.VAName, vaInfo.PVName,
						strconv.FormatBool(vaInfo.IsOrphan), vaInfo.AttachedNode, strconv.FormatBool(vaInfo.AttachmentStatus))
				}
			}

		} else {
			if res.TotalOrphanVA > 0 {
				fmt.Fprintf(w, "\nORPHAN_VOLUME_ATTACHMENT_NAME\tPV_NAME\tATTACH_NODE\tIS_ATTACHED\n")
				for _, vaInfo := range res.VAInfo {
					if vaInfo.IsOrphan {
						fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", vaInfo.VAName, vaInfo.PVName, vaInfo.AttachedNode, strconv.FormatBool(vaInfo.AttachmentStatus))
					}
				}
			}
		}
		w.Flush()
		fmt.Printf("\n----------------------- Summary ------------------------------\n")
		fmt.Printf("Total volume attachment CRs found: %d\n", res.TotalVA)
		fmt.Printf("Total orphan volume attachment CRs found: %d\n", res.TotalOrphanVA)
	},
}

func InitLs() {
	lsCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file (alternatively use CNSCTL_KUBECONFIG env variable)")
	lsCmd.PersistentFlags().BoolVarP(&all, "all", "a", false, "Show orphan and used volume attachment CRs in the Kubernetes cluster")
	ovaCmd.AddCommand(lsCmd)
}

func validateLsFlags() {
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set for 'ls' sub-command")
		os.Exit(1)
	}
}
