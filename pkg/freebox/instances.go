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
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	freego "github.com/nikolalohinski/free-go/client"
	"github.com/nikolalohinski/free-go/types"
)

// FreeboxInstances implements cloudprovider.InstancesV2.
//
// The core responsibility is InstanceMetadata, which maps a Kubernetes node to
// its Freebox VM ID and returns a providerID of the form "freebox://<vmID>".
//
// Lookup flow:
//
//	node.status.addresses[InternalIP]
//	    → GET /api/<version>/lan/browser/pub/
//	      match host by IP → obtain MAC address
//	    → GET /api/<version>/vm/
//	      match VM by MAC → obtain VM ID
//	    → providerID = fmt.Sprintf("freebox://%d", vmID)
type FreeboxInstances struct {
	client freego.Client
}

// newFreeboxInstances creates a FreeboxInstances using the given Config.
// The free-go client handles session management internally: it will call
// Login() automatically (challenge/response HMAC-SHA1) whenever a session is
// absent or expired.
func newFreeboxInstances(cfg *Config) (*FreeboxInstances, error) {
	c, err := freego.New(cfg.Endpoint, cfg.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to create Freebox client: %w", err)
	}
	c = c.WithAppID(cfg.AppID).WithPrivateToken(cfg.AppToken)
	return &FreeboxInstances{client: c}, nil
}

// nodeInternalIP returns the first InternalIP address of the node, or an empty
// string if none is present.
func nodeInternalIP(node *v1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

// vmIDForIP resolves a node IP address to a Freebox VM ID.
//
//  1. Query the LAN browser (interface "pub") to find the MAC address of the
//     host whose L3 connectivity matches the given IP.
//  2. List all virtual machines and match by MAC.
func (f *FreeboxInstances) vmIDForIP(ctx context.Context, ip string) (int64, error) {
	// Step 1: LAN browser → find MAC for this IP.
	hosts, err := f.client.GetLanInterface(ctx, "pub")
	if err != nil {
		return 0, fmt.Errorf("failed to list LAN hosts: %w", err)
	}

	mac := ""
	for _, h := range hosts {
		for _, l3 := range h.L3Connectivities {
			if l3.Address == ip {
				// LanInterfaceHost.L2Ident.ID holds the MAC address.
				mac = h.L2Ident.ID
				break
			}
		}
		if mac != "" {
			break
		}
	}
	if mac == "" {
		return 0, fmt.Errorf("no LAN host found with IP %s", ip)
	}

	// Step 2: VM list → find VM with matching MAC.
	vms, err := f.client.ListVirtualMachines(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list virtual machines: %w", err)
	}
	for _, vm := range vms {
		if strings.EqualFold(vm.Mac, mac) {
			return vm.ID, nil
		}
	}
	return 0, fmt.Errorf("no VM found with MAC %s (IP %s)", mac, ip)
}

// InstanceExists returns true if a Freebox VM can be found for the node.
// Returns false (not an error) when the VM is simply not found.
func (f *FreeboxInstances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	ip := nodeInternalIP(node)
	if ip == "" {
		return false, nil
	}
	_, err := f.vmIDForIP(ctx, ip)
	if err != nil {
		klog.V(4).InfoS("InstanceExists: VM not found", "node", node.Name, "err", err)
		return false, nil
	}
	return true, nil
}

// InstanceShutdown returns true if the Freebox VM is not in the "running"
// state.
func (f *FreeboxInstances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	ip := nodeInternalIP(node)
	if ip == "" {
		return false, nil
	}
	vmID, err := f.vmIDForIP(ctx, ip)
	if err != nil {
		return false, fmt.Errorf("InstanceShutdown: %w", err)
	}
	vm, err := f.client.GetVirtualMachine(ctx, vmID)
	if err != nil {
		return false, fmt.Errorf("failed to get VM %d: %w", vmID, err)
	}
	return vm.Status != types.RunningStatus, nil
}

// InstanceMetadata returns the cloud metadata for the node, most importantly
// its providerID ("freebox://<vmID>").  This is the method that the CCM
// framework calls to populate node.spec.providerID.
func (f *FreeboxInstances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	ip := nodeInternalIP(node)
	if ip == "" {
		return nil, fmt.Errorf("node %s has no InternalIP address", node.Name)
	}
	vmID, err := f.vmIDForIP(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("node %s (IP %s): %w", node.Name, ip, err)
	}
	providerID := fmt.Sprintf("freebox://%d", vmID)
	klog.InfoS("Resolved node to Freebox VM", "node", node.Name, "ip", ip, "providerID", providerID)
	return &cloudprovider.InstanceMetadata{
		ProviderID: providerID,
	}, nil
}
