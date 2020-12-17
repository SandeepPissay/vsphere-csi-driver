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

	"github.com/spf13/cobra"
)

// cleanupCmd represents the cleanup command
var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Identifies orphan volume attachment CRs and deletes them",
	Long:  "Identifies orphan volume attachment CRs and deletes them",
	Run: func(cmd *cobra.Command, args []string) {
		validateCleanupFlags()

		if len(args) != 0 {
			fmt.Printf("error: no arguments allowed for cleanup\n")
			os.Exit(1)
		}
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
		deleteCount, err := ov.DeleteVolumeAttachments(ctx, kubeClient, res)
		if err != nil {
			fmt.Printf("Unable to delete orphan volume attachments. Err: %+v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n----------------------- Summary ------------------------------\n")
		fmt.Printf("Total volume attachment CRs found: %d\n", res.TotalVA)
		fmt.Printf("Total orphan volume attachment CRs found: %d\n", res.TotalOrphanVA)
		fmt.Printf("Total orphan volume attachment deleted: %d\n", deleteCount)
	},
}

func InitCleanup() {
	cleanupCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file (alternatively use CNSCTL_KUBECONFIG env variable)")
	ovaCmd.AddCommand(cleanupCmd)
}

func validateCleanupFlags() {
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set for 'ls' sub-command")
		os.Exit(1)
	}
}
