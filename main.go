/*
Copyright 2025.

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

// freebox-cloud-controller-manager is a Kubernetes External Cloud Controller
// Manager for Freebox infrastructure.  Its primary purpose is to set
// node.spec.providerID on workload cluster nodes by resolving node IP addresses
// to Freebox VM IDs via the Freebox LAN browser API.
package main

import (
	"os"

	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	// Import the Freebox provider so its init() registers it with the
	// cloudprovider global registry.
	_ "github.com/mcanevet/freebox-cloud-controller-manager/pkg/freebox"
)

func main() {
	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	fss := cliflag.NamedFlagSets{}
	command := app.NewCloudControllerManagerCommand(
		ccmOptions,
		cloudInitializer,
		app.DefaultInitFuncConstructors,
		names.CCMControllerAliases(),
		fss,
		wait.NeverStop,
	)
	code := cli.Run(command)
	os.Exit(code)
}

// cloudInitializer constructs the Freebox cloud provider via the global
// registry (populated by the freebox package's init function).
func cloudInitializer(completedConfig *config.CompletedConfig) cloudprovider.Interface {
	cloudConfig := completedConfig.ComponentConfig.KubeCloudShared.CloudProvider

	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("cloud provider is nil")
	}
	return cloud
}
