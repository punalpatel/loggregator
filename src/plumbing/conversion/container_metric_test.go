package conversion_test

import (
	"plumbing/conversion"
	v2 "plumbing/v2"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("ContainerMetric", func() {
	Context("given a v3 envelope", func() {
		It("converts to a v2 protobuf", func() {
			envelope := &v2.Envelope{
				SourceUuid: "some-id",
				Message: &v2.Envelope_Gauge{
					Gauge: &v2.Gauge{
						Metrics: map[string]*v2.GaugeValue{
							"instance_index": {
								Unit:  "index",
								Value: 123,
							},
							"cpu": {
								Unit:  "percentage",
								Value: 11,
							},
							"memory": {
								Unit:  "bytes",
								Value: 13,
							},
							"disk": {
								Unit:  "bytes",
								Value: 15,
							},
							"memory_quota": {
								Unit:  "bytes",
								Value: 17,
							},
							"disk_quota": {
								Unit:  "bytes",
								Value: 19,
							},
						},
					},
				},
			}

			Expect(*conversion.ToV1(envelope)).To(MatchFields(IgnoreExtras, Fields{
				"EventType": Equal(events.Envelope_ContainerMetric.Enum()),
				"ContainerMetric": Equal(&events.ContainerMetric{
					ApplicationId:    proto.String("some-id"),
					InstanceIndex:    proto.Int32(123),
					CpuPercentage:    proto.Float64(11),
					MemoryBytes:      proto.Uint64(13),
					DiskBytes:        proto.Uint64(15),
					MemoryBytesQuota: proto.Uint64(17),
					DiskBytesQuota:   proto.Uint64(19),
				}),
			}))
		})

		DescribeTable("it is resilient to malformed envelopes", func(v2e *v2.Envelope) {
			Expect(conversion.ToV1(v2e)).To(BeNil())
		},
			Entry("bare envelope", &v2.Envelope{}),
			Entry("partial container metric", &v2.Envelope{
				Message: &v2.Envelope_Gauge{
					Gauge: &v2.Gauge{
						Metrics: map[string]*v2.GaugeValue{
							"cpu": {
								Value: 99,
							},
							"memory": {
								Value: 101,
							},
						},
					},
				},
			}),
			Entry("with empty fields", &v2.Envelope{
				Message: &v2.Envelope_Gauge{
					Gauge: &v2.Gauge{
						Metrics: map[string]*v2.GaugeValue{
							"instance_index": nil,
							"cpu":            nil,
							"memory":         nil,
							"disk":           nil,
							"memory_quota":   nil,
							"disk_quota":     nil,
						},
					},
				},
			}),
		)
	})
})
