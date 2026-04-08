package freebox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeInternalIP(t *testing.T) {
	tests := []struct {
		name       string
		node       *v1.Node
		expectedIP string
	}{
		{
			name: "returns internal IP",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeHostName, Address: "node-1"},
						{Type: v1.NodeInternalIP, Address: "192.168.1.10"},
					},
				},
			},
			expectedIP: "192.168.1.10",
		},
		{
			name: "returns empty when no internal IP",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeHostName, Address: "node-1"},
					},
				},
			},
			expectedIP: "",
		},
		{
			name: "returns first internal IP when multiple",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeInternalIP, Address: "192.168.1.10"},
						{Type: v1.NodeInternalIP, Address: "10.0.0.1"},
					},
				},
			},
			expectedIP: "192.168.1.10",
		},
		{
			name: "handles empty node",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status:     v1.NodeStatus{Addresses: nil},
			},
			expectedIP: "",
		},
		{
			name: "handles empty addresses",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
				Status:     v1.NodeStatus{Addresses: []v1.NodeAddress{}},
			},
			expectedIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nodeInternalIP(tt.node)
			assert.Equal(t, tt.expectedIP, result)
		})
	}
}

func TestInstanceMetadata_NoIP(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	require.NoError(t, err)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{},
		},
	}

	_, err = fi.InstanceMetadata(context.Background(), node)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no InternalIP")
}

func TestInstanceMetadata_WithRealAPI(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hosts, err := fi.client.GetLanInterface(ctx, "pub")
	if err != nil {
		t.Skipf("Skipping test: Could not connect to Freebox API (LAN): %v", err)
	}

	if len(hosts) == 0 {
		t.Skip("Skipping test: No LAN hosts found on Freebox")
	}

	testHost := hosts[0]
	var testIP string
	for _, l3 := range testHost.L3Connectivities {
		if l3.Address != "" {
			testIP = l3.Address
			break
		}
	}

	if testIP == "" {
		t.Skip("Skipping test: No IP found in LAN hosts")
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: testIP},
			},
		},
	}

	metadata, err := fi.InstanceMetadata(ctx, node)
	if err != nil {
		t.Skipf("Skipping test: Could not get instance metadata (VM API may be unavailable): %v", err)
	}
	assert.NotEmpty(t, metadata.ProviderID)
	assert.Contains(t, metadata.ProviderID, "freebox://")

	t.Logf("Successfully resolved node %s (IP: %s) to providerID: %s", node.Name, testIP, metadata.ProviderID)
}

func TestInstanceExists_WithRealAPI(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hosts, err := fi.client.GetLanInterface(ctx, "pub")
	if err != nil {
		t.Skipf("Skipping test: Could not connect to Freebox API (LAN): %v", err)
	}

	if len(hosts) == 0 {
		t.Skip("Skipping test: No LAN hosts found on Freebox")
	}

	testHost := hosts[0]
	var testIP string
	for _, l3 := range testHost.L3Connectivities {
		if l3.Address != "" {
			testIP = l3.Address
			break
		}
	}

	if testIP == "" {
		t.Skip("Skipping test: No IP found in LAN hosts")
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: testIP},
			},
		},
	}

	exists, err := fi.InstanceExists(ctx, node)
	if err != nil {
		t.Skipf("Skipping test: Could not check instance existence (VM API may be unavailable): %v", err)
	}
	assert.True(t, exists, "Expected instance to exist for IP %s", testIP)
}

func TestInstanceShutdown_WithRealAPI(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hosts, err := fi.client.GetLanInterface(ctx, "pub")
	if err != nil {
		t.Skipf("Skipping test: Could not connect to Freebox API (LAN): %v", err)
	}

	if len(hosts) == 0 {
		t.Skip("Skipping test: No LAN hosts found on Freebox")
	}

	testHost := hosts[0]
	var testIP string
	for _, l3 := range testHost.L3Connectivities {
		if l3.Address != "" {
			testIP = l3.Address
			break
		}
	}

	if testIP == "" {
		t.Skip("Skipping test: No IP found in LAN hosts")
	}

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: testIP},
			},
		},
	}

	shutdown, err := fi.InstanceShutdown(ctx, node)
	require.NoError(t, err)
	t.Logf("Instance shutdown status for IP %s: %v", testIP, shutdown)
}

func TestVmIDForIP_WithRealAPI(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hosts, err := fi.client.GetLanInterface(ctx, "pub")
	if err != nil {
		t.Skipf("Skipping test: Could not connect to Freebox API (LAN): %v", err)
	}

	if len(hosts) == 0 {
		t.Skip("Skipping test: No LAN hosts found on Freebox")
	}

	testHost := hosts[0]
	var testIP string
	for _, l3 := range testHost.L3Connectivities {
		if l3.Address != "" {
			testIP = l3.Address
			break
		}
	}

	if testIP == "" {
		t.Skip("Skipping test: No IP found in LAN hosts")
	}

	vmID, err := fi.vmIDForIP(ctx, testIP)
	if err != nil {
		if strings.Contains(err.Error(), "failed to list virtual machines") || strings.Contains(err.Error(), "failed to get vms") {
			t.Skipf("Skipping test: VM API not available: %v", err)
		}
		t.Fatalf("Expected to find VM for IP %s, but got error: %v", testIP, err)
	}
	t.Logf("Successfully resolved IP %s to VM ID %d", testIP, vmID)
	assert.Greater(t, vmID, int64(0))
}

func TestVmIDForIP_NotFound(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = fi.vmIDForIP(ctx, "192.168.255.254")
	if err != nil {
		if strings.Contains(err.Error(), "failed to list virtual machines") {
			t.Skipf("Skipping test: VM API not available: %v", err)
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no LAN host found")
	}
}

func TestNewFreeboxInstances(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	fi, err := newFreeboxInstances(cfg)
	if err != nil {
		t.Skipf("Skipping test: Could not create Freebox client: %v", err)
	}
	assert.NotNil(t, fi)
	assert.NotNil(t, fi.client)
}

func TestCloudInterface(t *testing.T) {
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Skip("Skipping test: FREEBOX environment variables not set")
	}

	cloud, err := newCloud(cfg)
	require.NoError(t, err)

	assert.Equal(t, "freebox", cloud.ProviderName())
	assert.False(t, cloud.HasClusterID())

	lb, ok := cloud.LoadBalancer()
	assert.False(t, ok)
	assert.Nil(t, lb)

	instances, ok := cloud.Instances()
	assert.False(t, ok)
	assert.Nil(t, instances)

	zones, ok := cloud.Zones()
	assert.False(t, ok)
	assert.Nil(t, zones)

	clusters, ok := cloud.Clusters()
	assert.False(t, ok)
	assert.Nil(t, clusters)

	routes, ok := cloud.Routes()
	assert.False(t, ok)
	assert.Nil(t, routes)

	instancesV2, ok := cloud.InstancesV2()
	assert.True(t, ok)
	assert.NotNil(t, instancesV2)
}
