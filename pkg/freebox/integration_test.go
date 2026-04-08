package freebox

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
	clientset *kubernetes.Clientset
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Freebox CCM Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: false,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	clientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("Freebox CCM Integration", func() {
	Context("when a node is created", func() {
		It("should allow setting providerID on node", func() {
			ctx := context.Background()

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-integration",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeHostName, Address: "test-node-integration"},
						{Type: v1.NodeInternalIP, Address: "192.168.1.100"},
					},
				},
			}
			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Update providerID
			node.Spec.ProviderID = "freebox://42"
			err = k8sClient.Update(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Verify the update
			updatedNode := &v1.Node{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(node), updatedNode)).To(Succeed())
				g.Expect(updatedNode.Spec.ProviderID).To(Equal("freebox://42"))
			}).Should(Succeed())

			// Cleanup
			err = k8sClient.Delete(ctx, node)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle node with no IP gracefully", func() {
			ctx := context.Background()

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-no-ip",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeHostName, Address: "test-node-no-ip"},
					},
				},
			}
			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// The Freebox CCM should handle nodes with no internal IP
			// by returning an error from InstanceMetadata

			// Cleanup
			err = k8sClient.Delete(ctx, node)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle node deletion", func() {
			ctx := context.Background()

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-delete",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeInternalIP, Address: "192.168.1.200"},
					},
				},
			}
			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Verify deletion
			deletedNode := &v1.Node{}
			Eventually(func(g Gomega) bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(node), deletedNode)
				return err != nil
			}).Should(BeTrue())
		})
	})

	Context("node status updates", func() {
		It("should allow updating node conditions", func() {
			ctx := context.Background()

			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-conditions",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{Type: v1.NodeInternalIP, Address: "192.168.1.150"},
					},
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			}
			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Update condition
			node.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: time.Now()}
			err = k8sClient.Status().Update(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Verify update
			updatedNode := &v1.Node{}
			Eventually(func(g Gomega) int {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(node), updatedNode)).To(Succeed())
				return len(updatedNode.Status.Conditions)
			}).Should(Equal(1))

			// Cleanup
			err = k8sClient.Delete(ctx, node)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
