package api

import (
	"fmt"
	"log"
	"math/rand"
	clientpool "metron/clientpool/v1"
	"metron/config"
	"metron/eventwriter"
	"metron/legacyclientpool"
	"metron/networkreader"
	"metron/writers/dopplerforwarder"
	"metron/writers/eventmarshaller"
	"metron/writers/eventunmarshaller"
	"metron/writers/messageaggregator"
	"metron/writers/tagger"
	"plumbing"
	"time"

	"github.com/cloudfoundry/dropsonde/metric_sender"
	"github.com/cloudfoundry/dropsonde/metricbatcher"
	"github.com/cloudfoundry/dropsonde/metrics"
	"github.com/cloudfoundry/dropsonde/runtime_stats"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type AppV1 struct{}

func (a *AppV1) Start(config *config.Config) {
	if config.DisableUDP {
		return
	}

	statsStopChan := make(chan struct{})
	batcher, eventWriter := a.initializeMetrics(config, statsStopChan)

	log.Print("Startup: Setting up the Metron agent")
	marshaller, err := a.initializeV1DopplerPool(config, batcher)
	if err != nil {
		log.Panic(fmt.Errorf("Could not initialize doppler connection pool: %s", err))
	}

	messageTagger := tagger.New(config.Deployment, config.Job, config.Index, marshaller)
	aggregator := messageaggregator.New(messageTagger)
	eventWriter.SetWriter(aggregator)

	dropsondeUnmarshaller := eventunmarshaller.New(aggregator, batcher)
	metronAddress := fmt.Sprintf("127.0.0.1:%d", config.IncomingUDPPort)
	dropsondeReader, err := networkreader.New(metronAddress, "dropsondeAgentListener", dropsondeUnmarshaller)
	if err != nil {
		log.Panic(fmt.Errorf("Failed to listen on %s: %s", metronAddress, err))
	}

	log.Print("metron v1 API started")
	dropsondeReader.Start()
}

func (a *AppV1) initializeMetrics(config *config.Config, stopChan chan struct{}) (*metricbatcher.MetricBatcher, *eventwriter.EventWriter) {
	eventWriter := eventwriter.New("MetronAgent")
	metricSender := metric_sender.NewMetricSender(eventWriter)
	metricBatcher := metricbatcher.New(metricSender, time.Duration(config.MetricBatchIntervalMilliseconds)*time.Millisecond)
	metrics.Initialize(metricSender, metricBatcher)

	stats := runtime_stats.NewRuntimeStats(eventWriter, time.Duration(config.RuntimeStatsIntervalMilliseconds)*time.Millisecond)
	go stats.Run(stopChan)
	return metricBatcher, eventWriter
}

func (a *AppV1) initializeV1DopplerPool(conf *config.Config, batcher *metricbatcher.MetricBatcher) (*eventmarshaller.EventMarshaller, error) {
	pools := a.setupGRPC(conf)

	// TODO: delete this legacy pool stuff when UDP goes away
	legacyPool := legacyclientpool.New(conf.DopplerAddrUDP, 100, 5*time.Second)
	udpWrapper := dopplerforwarder.NewUDPWrapper(legacyPool, []byte(conf.SharedSecret))
	pools = append(pools, udpWrapper)

	combinedPool := legacyclientpool.NewCombinedPool(pools...)

	marshaller := eventmarshaller.New(batcher)
	marshaller.SetWriter(combinedPool)

	return marshaller, nil
}

func (a *AppV1) setupGRPC(conf *config.Config) []legacyclientpool.Pool {
	tlsConfig, err := plumbing.NewMutualTLSConfig(
		conf.GRPC.CertFile,
		conf.GRPC.KeyFile,
		conf.GRPC.CAFile,
		"doppler",
	)
	if err != nil {
		log.Printf("Failed to load TLS config: %s", err)
		return nil
	}

	connector := clientpool.MakeGRPCConnector(
		conf.DopplerAddr,
		conf.Zone,
		grpc.Dial,
		plumbing.NewDopplerIngestorClient,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)

	var connManagers []clientpool.Conn
	for i := 0; i < 5; i++ {
		connManagers = append(connManagers, clientpool.NewConnManager(connector, 10000+rand.Int63n(1000)))
	}

	pool := clientpool.New(connManagers...)
	grpcWrapper := dopplerforwarder.NewGRPCWrapper(pool)
	return []legacyclientpool.Pool{grpcWrapper}
}
