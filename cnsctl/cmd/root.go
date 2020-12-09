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
	"github.com/spf13/cobra"
	"os"

	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cnsctl",
	Short: "CLI tool for CNS-CSI.",
	Long:  "A fast CLI based tool to interact with CNS-CSI for various storage control operations.",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func InitViper() {
	viper.SetEnvPrefix("cnsctl")
	viper.BindEnv("datacenter")
	viper.BindEnv("datastores")
	viper.BindEnv("kubeconfig")
	viper.AutomaticEnv() // read in environment variables that match
}

func InitRoot() {
	InitViper()
	InitOv()
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "kubeconfig", "k", viper.GetString("kubeconfig"), "kubeconfig file")
}

func initConfig() {
	if cfgFile == "" {
		fmt.Println("error: kubeconfig flag or CNSCTL_KUBECONFIG env variable not set!")
		os.Exit(1)
	}
}
