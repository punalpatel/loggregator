package main

import (
	"crypto/sha1"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudfoundry/storeadapter"
)

type EtcdSyslogDrainStore struct {
	storeAdapter storeadapter.StoreAdapter
	ttl          time.Duration
}

func NewEtcdSyslogDrainStore(storeAdapter storeadapter.StoreAdapter, ttl time.Duration) *EtcdSyslogDrainStore {
	return &EtcdSyslogDrainStore{
		storeAdapter: storeAdapter,
		ttl:          ttl,
	}
}

func (store *EtcdSyslogDrainStore) UpdateDrains(appDrainUrlMap map[AppID][]DrainURL) error {

	for appId, drainUrls := range appDrainUrlMap {
		err := store.updateAppDrains(appId, drainUrls)
		if err != nil {
			return err
		}
	}

	return nil
}

func (store *EtcdSyslogDrainStore) updateAppDrains(appId AppID, drainUrls []DrainURL) error {
	var nodes []storeadapter.StoreNode

	for _, drainUrl := range drainUrls {

		if strings.TrimSpace(string(drainUrl)) == "" {
			log.Printf("UpdateDrains: attempted to add whitespace-only drain url '%s' for app %s. Skipping.", drainUrl, appId)
			continue
		}

		log.Printf("UpdateDrains: adding drain %s to app %s", drainUrl, appId)
		node := storeadapter.StoreNode{
			Key:   drainKey(appId, drainUrl),
			Value: []byte(drainUrl),
			TTL:   uint64(store.ttl.Seconds()),
		}

		nodes = append(nodes, node)
	}

	if len(nodes) > 0 {
		err := store.storeAdapter.SetMulti(nodes)
		if err != nil {
			return err
		}
	}

	return nil
}

func appKey(appId AppID) string {
	return fmt.Sprintf("/loggregator/services/%s", appId)
}

func drainKey(appId AppID, drainUrl DrainURL) string {
	hash := sha1.Sum([]byte(drainUrl))
	return fmt.Sprintf("%s/%x", appKey(appId), hash)
}
