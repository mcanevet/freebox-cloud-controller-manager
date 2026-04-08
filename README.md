# Freebox Cloud Controller Manager

A Kubernetes External Cloud Controller Manager (CCM) for Freebox infrastructure. Its primary purpose is to set `node.spec.providerID` on workload cluster nodes by resolving node IP addresses to Freebox VM IDs via the Freebox API.

## Overview

When running a Kubernetes cluster on Freebox Virtual Machines, the kubelet needs to know which VM each node corresponds to. This CCM bridges Kubernetes nodes to Freebox by:

1. Reading the node's InternalIP from `node.status.addresses`
2. Querying the Freebox LAN browser API to find the MAC address matching that IP
3. Querying the Freebox VM API to find the VM with that MAC address
4. Setting `node.spec.providerID` to `freebox://<vmID>`

## Requirements

- A Kubernetes cluster running on Freebox VMs (Talos Linux recommended)
- Freebox API token with LAN and VM access permissions
- Kubernetes with `--cloud-provider=external` on workers

## Quick Start

### 1. Create Freebox API Token

Follow the [Freebox API documentation](https://dev.freebox.fr/sdk/oauth/) to generate an application token.

### 2. Deploy the CCM

```bash
# Create the secret with your Freebox credentials
kubectl create secret generic freebox-cloud-config \
  --namespace=kube-system \
  --from-literal=FREEBOX_ENDPOINT=http://mafreebox.freebox.fr \
  --from-literal=FREEBOX_VERSION=v8 \
  --from-literal=FREEBOX_APP_ID=your-app-id \
  --from-literal=FREEBOX_TOKEN=your-app-token

# Deploy the CCM
kubectl apply -f deploy/
```

### 3. Configure Talos

Add to your Talos machine config:

```yaml
machine:
  kubelet:
    extraArgs:
      cloud-provider: external
cluster:
  externalCloudProvider:
    enabled: true
```

## Environment Variables

The CCM reads configuration from these environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `FREEBOX_ENDPOINT` | Freebox API base URL | `http://mafreebox.freebox.fr` |
| `FREEBOX_VERSION` | API version | `v8` |
| `FREEBOX_APP_ID` | Application identifier | `fr.freebox.ccm` |
| `FREEBOX_TOKEN` | Private application token | `xxx...` |

## Architecture

### Components

- **pkg/freebox/cloud.go**: Provider registration and interface implementation
- **pkg/freebox/config.go**: Environment variable configuration
- **pkg/freebox/instances.go**: InstancesV2 implementation (core logic)

### IP to VM Resolution Flow

```
node.status.addresses[InternalIP]
    │
    ▼
GET /api/<version>/lan/browser/pub/
    │  returns []LanInterfaceHost{MAC, IP, ...}
    ▼
Match host by IP → obtain MAC address
    │
    ▼
GET /api/<version>/vm/
    │  returns []VirtualMachine{ID, Mac, Status, ...}
    ▼
Match VM by MAC → obtain VM ID
    │
    ▼
providerID = "freebox://<vmID>"
```

### ProviderID Format

Nodes managed by this CCM will have:
- `node.spec.providerID` = `freebox://<vmID>` (e.g., `freebox://42`)

## Development

### Building

```bash
# Build the binary
make build

# Build container image
make docker-build
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests (requires envtest)
make test-integration
```

### Test Coverage

- **Unit tests**: Config loading, node IP extraction
- **Integration tests**: Kubernetes API interactions with envtest
- **Real API tests**: Connect to actual Freebox API (skip when credentials unavailable)

### Code Linting

```bash
make lint
```

## Deployment

### RBAC

The CCM requires these cluster-level permissions:

```yaml
rules:
  - apiGroups: [""]
    resources: [nodes]
    verbs: [get, list, watch, patch, update]
  - apiGroups: [""]
    resources: [nodes/status]
    verbs: [patch, update]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch, update]
```

### DaemonSet Configuration

- Runs on control-plane nodes only
- Uses `hostNetwork: true` to communicate with the API server
- `--controllers=cloud-node` to only run the cloud-node controller

### Tolerations

The CCM tolerates:
- `node-role.kubernetes.io/control-plane:NoSchedule` - runs on control plane
- `node.cloudprovider.kubernetes.io/uninitialized:NoSchedule` - allows initialization before CCM starts

## References

- [Kubernetes CCM Documentation](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Freebox API Documentation](https://dev.freebox.fr/sdk/)
- [free-go Library](https://github.com/nikolalohinski/free-go)

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.