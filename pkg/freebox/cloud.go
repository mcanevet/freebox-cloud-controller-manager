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

package freebox

import (
	"io"

	cloudprovider "k8s.io/cloud-provider"
)

// ProviderName is the name used to register this cloud provider with the CCM
// framework and must match the --cloud-provider flag value.
const ProviderName = "freebox"

// Cloud implements cloudprovider.Interface for the Freebox infrastructure.
// Only InstancesV2 is implemented; all other sub-interfaces return nil/false.
type Cloud struct {
	cfg       *Config
	instances *FreeboxInstances
}

// newCloud creates a Cloud from the provided Config.
func newCloud(cfg *Config) (*Cloud, error) {
	inst, err := newFreeboxInstances(cfg)
	if err != nil {
		return nil, err
	}
	return &Cloud{cfg: cfg, instances: inst}, nil
}

// init registers the Freebox cloud provider with the global CCM registry.
// The blank io.Reader argument is not used — we load config from env.
func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(_ io.Reader) (cloudprovider.Interface, error) {
		cfg, err := ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		return newCloud(cfg)
	})
}

// Initialize is called by the CCM after the provider is constructed.
// We have nothing to do here; all initialization is done in newCloud.
func (c *Cloud) Initialize(_ cloudprovider.ControllerClientBuilder, _ <-chan struct{}) {}

// ProviderName returns the cloud provider identifier.
func (c *Cloud) ProviderName() string { return ProviderName }

// HasClusterID indicates whether the cloud has a concept of cluster ID.
// The Freebox does not, so we return false.
func (c *Cloud) HasClusterID() bool { return false }

// LoadBalancer returns nil because the Freebox CCM does not manage load
// balancers.
func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) { return nil, false }

// Instances returns nil because we implement the newer InstancesV2 interface
// instead.
func (c *Cloud) Instances() (cloudprovider.Instances, bool) { return nil, false }

// Zones returns nil because the Freebox does not expose zone information.
func (c *Cloud) Zones() (cloudprovider.Zones, bool) { return nil, false }

// Clusters returns nil because the Freebox CCM does not manage clusters.
func (c *Cloud) Clusters() (cloudprovider.Clusters, bool) { return nil, false }

// Routes returns nil because the Freebox CCM does not manage routes.
func (c *Cloud) Routes() (cloudprovider.Routes, bool) { return nil, false }

// InstancesV2 returns the InstancesV2 implementation, which is the only
// sub-interface this CCM actually implements.
func (c *Cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}
