package main_test

import (
	"fmt"
	"time"

	"github.com/cloudfoundry/loggregatorlib/loggertesthelper"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	"loggregator"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
)

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdPort int

var _ = BeforeSuite(func() {
	etcdPort = 5800 + (config.GinkgoConfig.ParallelNode-1)*10
	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)
	etcdRunner.Start()
})

var _ = AfterSuite(func() {
	etcdRunner.Adapter().Disconnect()
	etcdRunner.Stop()
})
var _ = BeforeEach(func() {
	adapter := etcdRunner.Adapter()
	adapter.Disconnect()
	etcdRunner.Reset()
	adapter.Connect()
})

var _ = Describe("Etcd Integration tests", func() {
	var config main.Config
	var stopHeartbeats chan (chan bool)

	BeforeEach(func() {
		stopHeartbeats = nil

		config = main.Config{
			JobName: "loggregator_z1",
			Index:   0,
			EtcdMaxConcurrentRequests: 1,
			EtcdUrls:                  []string{fmt.Sprintf("http://127.0.0.1:%d", etcdPort)},
			Zone:                      "z1",
		}
	})

	AfterEach(func() {
		if stopHeartbeats != nil {
			heartbeatsStopped := make(chan bool)
			stopHeartbeats <- heartbeatsStopped
			<-heartbeatsStopped
		}
	})

	Describe("Heartbeats", func() {
		It("arrives safely in etcd", func() {
			adapter := etcdRunner.Adapter()

			Consistently(func() error {
				_, err := adapter.Get("healthstatus/loggregator/z1/loggregator_z1/0")
				return err
			}).Should(HaveOccurred())

			stopHeartbeats = main.StartHeartbeats(time.Second, &config, loggertesthelper.Logger())

			Eventually(func() error {
				_, err := adapter.Get("healthstatus/loggregator/z1/loggregator_z1/0")
				return err
			}).ShouldNot(HaveOccurred())
		})

		It("has a 10 sec TTL", func() {
			stopHeartbeats = main.StartHeartbeats(time.Second, &config, loggertesthelper.Logger())
			adapter := etcdRunner.Adapter()

			Eventually(func() uint64 {
				node, _ := adapter.Get("healthstatus/loggregator/z1/loggregator_z1/0")
				return node.TTL
			}).Should(BeNumerically(">", 0))
		})

		It("updates the value periodically", func() {
			stopHeartbeats = main.StartHeartbeats(time.Second, &config, loggertesthelper.Logger())
			adapter := etcdRunner.Adapter()

			var indices []uint64
			var index uint64
			Eventually(func() uint64 {
				node, _ := adapter.Get("healthstatus/loggregator/z1/loggregator_z1/0")
				index = node.Index
				return node.Index
			}).Should(BeNumerically(">", 0))
			indices = append(indices, index)

			for i := 0; i < 3; i++ {
				Eventually(func() uint64 {
					node, _ := adapter.Get("healthstatus/loggregator/z1/loggregator_z1/0")
					index = node.Index
					return node.Index
				}).Should(BeNumerically(">", indices[i]))
				indices = append(indices, index)

			}
		})
	})
})
