# Freebox Cloud Controller Manager — Implementation Plan

## Goal

Build a Kubernetes External Cloud Controller Manager (CCM) that runs inside a
workload cluster and sets `node.spec.providerID` on each node by resolving the
node's IP address to a Freebox VM ID via the Freebox LAN browser API.

**Learning objective**: understand how to write a CCM from scratch using the
`k8s.io/cloud-provider` framework.

---

## Background

### What is a CCM?

A Cloud Controller Manager is an optional Kubernetes control-plane component that
bridges the cluster to an underlying infrastructure provider.  It runs as a
`DaemonSet` (on control-plane nodes) in `kube-system`.

The upstream `k8s.io/cloud-provider` package defines the `cloud.Interface`:

```go
type Interface interface {
    Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{})
    LoadBalancer() (LoadBalancer, bool)
    Instances() (Instances, bool)       // deprecated
    InstancesV2() (InstancesV2, bool)   // preferred
    Zones() (Zones, bool)               // deprecated
    Clusters() (Clusters, bool)
    Routes() (Routes, bool)
    ProviderName() string
    HasClusterID() bool
}
```

For a minimal CCM that only sets `providerID` we implement:
- `InstancesV2() (InstancesV2, bool)` → return our impl, `true`
- All other methods → return `nil, false` or `"freebox"` / `false`

### The `InstancesV2` interface

```go
type InstancesV2 interface {
    InstanceExists(ctx context.Context, node *v1.Node) (bool, error)
    InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error)
    InstanceMetadata(ctx context.Context, node *v1.Node) (*InstanceMetadata, error)
}
```

`InstanceMetadata` is the key method — it must return:

```go
&cloudprovider.InstanceMetadata{
    ProviderID: "freebox://42",
}
```

The CCM framework will write that value into `node.spec.providerID`.

### ProviderID format

```
freebox://<vmID>
```

Examples: `freebox://1`, `freebox://42`.

---

## Lookup flow

```
Node IP (from node.status.addresses[InternalIP])
    │
    ▼
GET /api/v8/lan/browser/pub/  (Freebox LAN browser)
    │  returns []LanInterfaceHost{MAC, IP, ...}
    ▼
Match host by IP  →  obtain MAC address
    │
    ▼
GET /api/v8/vm/  (list all VMs)
    │  returns []VirtualMachine{ID, Mac, Status, ...}
    ▼
Match VM by MAC  →  obtain VM ID
    │
    ▼
providerID = fmt.Sprintf("freebox://%d", vmID)
```

---

## Authentication

The Freebox API uses a two-step HMAC-SHA1 challenge/response session flow.

```
GET  /api/v8/login
     → { challenge: "abc123" }

password = HMAC-SHA1(privateToken, challenge)   // hex-encoded

POST /api/v8/login/session
     body: { app_id: "...", password: "..." }
     → { session_token: "xyz..." }

# All subsequent requests:
GET /api/v8/vm/
Headers: X-Fbx-App-Auth: xyz...
```

Sessions expire after ~30 minutes of inactivity.  The CCM will re-authenticate
on `401` responses.

Credentials are stored in a Kubernetes `Secret` in `kube-system` and mounted
into the pod as environment variables:

```
FREEBOX_ENDPOINT    e.g. https://mafreebox.freebox.fr
FREEBOX_API_VERSION e.g. v8
FREEBOX_APP_ID      e.g. fr.freebox.ccm
FREEBOX_APP_TOKEN   (the private token)
```

---

## Repository structure

```
freebox-cloud-controller-manager/
├── main.go                          # entry point, registers provider, starts CCM
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── plan.md                          # this file
├── pkg/
│   └── freebox/
│       ├── cloud.go                 # cloud.Interface implementation
│       ├── config.go                # credential loading from env
│       ├── instances.go             # InstancesV2 implementation
│       └── instances_test.go        # unit tests
└── deploy/
    ├── cloud-controller-manager.yaml   # DaemonSet, ClusterRole, etc.
    └── cloud-config-secret.yaml        # Secret template (values NOT committed)
```

---

## File-by-file implementation

### `go.mod`

```go
module github.com/mcanevet/freebox-cloud-controller-manager

go 1.25.0

require (
    github.com/nikolalohinski/free-go v1.10.0
    k8s.io/api v0.35.3
    k8s.io/apimachinery v0.35.3
    k8s.io/client-go v0.35.3
    k8s.io/cloud-provider v0.35.3
    k8s.io/component-base v0.35.0
    k8s.io/klog/v2 v2.130.1
)
```

---

### `pkg/freebox/config.go`

Load credentials from environment variables.

```go
package freebox

import (
    "fmt"
    "os"
)

// Config holds credentials and endpoint info for the Freebox API.
type Config struct {
    Endpoint   string // e.g. "https://mafreebox.freebox.fr"
    APIVersion string // e.g. "v8"
    AppID      string // e.g. "fr.freebox.ccm"
    AppToken   string // private token
}

// ConfigFromEnv reads the Freebox CCM configuration from environment variables.
func ConfigFromEnv() (*Config, error) {
    get := func(key string) (string, error) {
        v := os.Getenv(key)
        if v == "" {
            return "", fmt.Errorf("required env var %q is not set", key)
        }
        return v, nil
    }

    endpoint, err := get("FREEBOX_ENDPOINT")
    if err != nil {
        return nil, err
    }
    version, err := get("FREEBOX_API_VERSION")
    if err != nil {
        return nil, err
    }
    appID, err := get("FREEBOX_APP_ID")
    if err != nil {
        return nil, err
    }
    token, err := get("FREEBOX_APP_TOKEN")
    if err != nil {
        return nil, err
    }

    return &Config{
        Endpoint:   endpoint,
        APIVersion: version,
        AppID:      appID,
        AppToken:   token,
    }, nil
}
```

---

### `pkg/freebox/cloud.go`

Implements `cloud.Interface`.  Most methods return `nil, false` (not
implemented).  Only `InstancesV2` returns our real implementation.

```go
package freebox

import (
    "io"

    cloudprovider "k8s.io/cloud-provider"
)

const ProviderName = "freebox"

// Cloud implements cloudprovider.Interface for the Freebox API.
type Cloud struct {
    cfg       *Config
    instances *FreeboxInstances
}

// newCloud creates a Cloud from the given config.
func newCloud(cfg *Config) (*Cloud, error) {
    inst, err := newFreeboxInstances(cfg)
    if err != nil {
        return nil, err
    }
    return &Cloud{cfg: cfg, instances: inst}, nil
}

// init registers the Freebox cloud provider with the CCM framework.
func init() {
    cloudprovider.RegisterCloudProvider(ProviderName, func(_ io.Reader) (cloudprovider.Interface, error) {
        cfg, err := ConfigFromEnv()
        if err != nil {
            return nil, err
        }
        return newCloud(cfg)
    })
}

func (c *Cloud) Initialize(_ cloudprovider.ControllerClientBuilder, _ <-chan struct{}) {}
func (c *Cloud) ProviderName() string                                                  { return ProviderName }
func (c *Cloud) HasClusterID() bool                                                    { return false }
func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool)                      { return nil, false }
func (c *Cloud) Instances() (cloudprovider.Instances, bool)                            { return nil, false }
func (c *Cloud) Zones() (cloudprovider.Zones, bool)                                    { return nil, false }
func (c *Cloud) Clusters() (cloudprovider.Clusters, bool)                              { return nil, false }
func (c *Cloud) Routes() (cloudprovider.Routes, bool)                                  { return nil, false }

func (c *Cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
    return c.instances, true
}
```

---

### `pkg/freebox/instances.go`

The core logic: resolve a node's IP → LAN host MAC → VM ID → providerID.

```go
package freebox

import (
    "context"
    "fmt"
    "sync"
    "time"

    v1 "k8s.io/api/core/v1"
    cloudprovider "k8s.io/cloud-provider"
    "k8s.io/klog/v2"

    freego "github.com/nikolalohinski/free-go/client"
    "github.com/nikolalohinski/free-go/types"
)

// FreeboxInstances implements cloudprovider.InstancesV2.
type FreeboxInstances struct {
    cfg          *Config
    mu           sync.Mutex
    sessionToken string
    tokenExpiry  time.Time
    client       freego.Client
}

func newFreeboxInstances(cfg *Config) (*FreeboxInstances, error) {
    c, err := freego.New(cfg.Endpoint, cfg.APIVersion)
    if err != nil {
        return nil, fmt.Errorf("failed to create free-go client: %w", err)
    }
    return &FreeboxInstances{cfg: cfg, client: c}, nil
}

// ensureSession refreshes the session token if it has expired.
func (f *FreeboxInstances) ensureSession(ctx context.Context) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    if time.Now().Before(f.tokenExpiry) {
        return nil
    }
    token, err := GetSessionToken(f.cfg.Endpoint, f.cfg.APIVersion, f.cfg.AppID, f.cfg.AppToken)
    if err != nil {
        return fmt.Errorf("failed to open Freebox session: %w", err)
    }
    f.sessionToken = token
    f.tokenExpiry = time.Now().Add(25 * time.Minute) // renew before 30-min expiry
    if err := f.client.SetSession(token); err != nil {
        return fmt.Errorf("failed to set session on free-go client: %w", err)
    }
    return nil
}

// nodeIP returns the first InternalIP address of the given node.
func nodeIP(node *v1.Node) string {
    for _, addr := range node.Status.Addresses {
        if addr.Type == v1.NodeInternalIP {
            return addr.Address
        }
    }
    return ""
}

// vmIDForIP looks up the Freebox VM ID matching the given IP address.
// It queries the LAN browser to get the MAC, then matches against the VM list.
func (f *FreeboxInstances) vmIDForIP(ctx context.Context, ip string) (int64, error) {
    if err := f.ensureSession(ctx); err != nil {
        return 0, err
    }

    // Step 1: LAN browser → find MAC for this IP
    hosts, err := f.client.GetLanInterface(ctx, "pub")
    if err != nil {
        return 0, fmt.Errorf("failed to list LAN hosts: %w", err)
    }
    mac := ""
    for _, h := range hosts {
        if h.L3Connectivities != nil {
            for _, l3 := range h.L3Connectivities {
                if l3.Addr == ip {
                    mac = h.ID // free-go LAN host ID is the MAC
                    break
                }
            }
        }
        if mac != "" {
            break
        }
    }
    if mac == "" {
        return 0, fmt.Errorf("no LAN host found with IP %s", ip)
    }

    // Step 2: VM list → find VM with matching MAC
    vms, err := f.client.ListVirtualMachines(ctx)
    if err != nil {
        return 0, fmt.Errorf("failed to list virtual machines: %w", err)
    }
    for _, vm := range vms {
        if strings.EqualFold(string(vm.Mac), mac) {
            return vm.ID, nil
        }
    }
    return 0, fmt.Errorf("no VM found with MAC %s (IP %s)", mac, ip)
}

// InstanceExists returns true if the node maps to a known Freebox VM.
func (f *FreeboxInstances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
    ip := nodeIP(node)
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

// InstanceShutdown returns true if the VM is not in a running state.
func (f *FreeboxInstances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
    ip := nodeIP(node)
    if ip == "" {
        return false, nil
    }
    if err := f.ensureSession(ctx); err != nil {
        return false, err
    }
    vmID, err := f.vmIDForIP(ctx, ip)
    if err != nil {
        return false, err
    }
    vm, err := f.client.GetVirtualMachine(ctx, vmID)
    if err != nil {
        return false, fmt.Errorf("failed to get VM %d: %w", vmID, err)
    }
    return vm.Status != types.VirtualMachineStatusRunning, nil
}

// InstanceMetadata returns metadata for the node, including its providerID.
// This is the primary method called by the CCM to set node.spec.providerID.
func (f *FreeboxInstances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
    ip := nodeIP(node)
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
```

---

### `main.go`

Wire up the CCM framework.  The `k8s.io/cloud-provider` package provides
`app.NewCloudControllerManagerCommand` which does all the heavy lifting.

```go
package main

import (
    "os"

    "k8s.io/cloud-provider/app"
    "k8s.io/cloud-provider/app/config"
    "k8s.io/cloud-provider/names"
    "k8s.io/component-base/cli"
    _ "k8s.io/client-go/plugin/pkg/client/auth" // auth plugins

    // Import our provider so its init() registers it.
    _ "github.com/mcanevet/freebox-cloud-controller-manager/pkg/freebox"
)

func main() {
    controllerInitializers := app.DefaultInitFuncConstructors
    // We only need the cloud-node controller (sets providerID).
    // Remove node-route and service-lb controllers which we don't support.
    delete(controllerInitializers, names.CloudNodeLifecycleController)
    delete(controllerInitializers, names.ServiceLBController)
    delete(controllerInitializers, names.NodeRouteController)

    cmd := app.NewCloudControllerManagerCommand(
        app.NewDefaultComponentConfig(),
        app.DefaultAuthFuncConstructor,
        controllerInitializers,
        config.DefaultGenericControlPlaneConfigFuncConstructor,
        []string{},
    )
    code := cli.Run(cmd)
    os.Exit(code)
}
```

---

### `deploy/cloud-config-secret.yaml`

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: freebox-cloud-config
  namespace: kube-system
type: Opaque
stringData:
  FREEBOX_ENDPOINT: "https://mafreebox.freebox.fr"
  FREEBOX_API_VERSION: "v8"
  FREEBOX_APP_ID: "fr.freebox.ccm"
  FREEBOX_APP_TOKEN: "<your-private-token>"  # fill in before applying
```

### `deploy/cloud-controller-manager.yaml`

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: freebox-cloud-controller-manager
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: freebox-cloud-controller-manager
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
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: freebox-cloud-controller-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: freebox-cloud-controller-manager
subjects:
  - kind: ServiceAccount
    name: freebox-cloud-controller-manager
    namespace: kube-system
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: freebox-cloud-controller-manager
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: freebox-cloud-controller-manager
  template:
    metadata:
      labels:
        app: freebox-cloud-controller-manager
    spec:
      serviceAccountName: freebox-cloud-controller-manager
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          effect: NoSchedule
        - key: node.cloudprovider.kubernetes.io/uninitialized
          value: "true"
          effect: NoSchedule
      nodeSelector:
        node-role.kubernetes.io/control-plane: ""
      hostNetwork: true
      containers:
        - name: freebox-cloud-controller-manager
          image: ghcr.io/mcanevet/freebox-cloud-controller-manager:latest
          command:
            - /freebox-cloud-controller-manager
            - --cloud-provider=freebox
            - --leader-elect=true
            - --use-service-account-credentials=true
            - --controllers=cloud-node
          envFrom:
            - secretRef:
                name: freebox-cloud-config
          resources:
            requests:
              cpu: 50m
              memory: 50Mi
```

---

### `Makefile`

```makefile
IMG ?= ghcr.io/mcanevet/freebox-cloud-controller-manager:latest

.PHONY: build
build:
	go build -o bin/freebox-cloud-controller-manager ./...

.PHONY: test
test:
	go test ./...

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)
```

### `Dockerfile`

```dockerfile
FROM golang:1.25 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o freebox-cloud-controller-manager .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/freebox-cloud-controller-manager /freebox-cloud-controller-manager
ENTRYPOINT ["/freebox-cloud-controller-manager"]
```

---

## Integration with Talos

For the CCM to work, kubelet must be started with `--cloud-provider=external`.
In the Talos machine config patch (e.g. `talos-example/controlplane.yaml`),
uncomment:

```yaml
- op: add
  path: /machine/kubelet/extraArgs
  value:
    cloud-provider: external
    rotate-server-certificates: "true"
- op: add
  path: /cluster/externalCloudProvider
  value:
    enabled: true
    manifests:
      - https://raw.githubusercontent.com/mcanevet/freebox-cloud-controller-manager/main/deploy/cloud-config-secret.yaml
      - https://raw.githubusercontent.com/mcanevet/freebox-cloud-controller-manager/main/deploy/cloud-controller-manager.yaml
```

Without `--cloud-provider=external`, kubelet marks each node as
`node.cloudprovider.kubernetes.io/uninitialized=true:NoSchedule` and waits for
the CCM, but the CCM cannot see the node's IP properly — or conversely, if
kubelet is NOT told about an external provider, it never adds the taint and the
CCM's node controller does nothing useful.

---

## Implementation order

1. `go.mod` / `go.sum` — `go mod init` + `go mod tidy`
2. `pkg/freebox/config.go` — credential loading (trivial)
3. `pkg/freebox/instances.go` — core logic (IP → MAC → vmID)
4. `pkg/freebox/cloud.go` — `cloud.Interface` wrapper + `init()`
5. `main.go` — wire up CCM command
6. `pkg/freebox/instances_test.go` — unit tests with mocks
7. `deploy/` — Kubernetes manifests
8. `Makefile` + `Dockerfile`

---

## Open questions / future work

- Should the CCM also set `node.status.addresses`?  Currently the plan is
  providerID only (YAGNI).
- Session expiry is handled by a fixed 25-minute timer.  A more robust approach
  would catch `401` responses and re-authenticate.  Add only when needed.
- The `free-go` client API for `GetLanInterface` returns `LanInterfaceHost`.
  Need to verify the exact field names (`ID`, `L3Connectivities`, `Addr`) match
  what free-go v1.10.0 actually exposes — check before writing `instances.go`.
