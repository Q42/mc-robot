// +build mock

// Package datasource defines here a Mock for the PubSub datasource for local testing
package datasource

import (
	"encoding/json"
	"log"
	"time"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
)

const broadcastRequestTopic = "broadcastRequest"

type pubSubDatasource struct {
}

var _ ExternalSource = &pubSubDatasource{}

var savedCb func(jsonData []byte, source string)

func (*pubSubDatasource) Subscribe(topic string, cb func(jsonData []byte, source string)) {
	savedCb = cb
}

func (*pubSubDatasource) Unsubscribe(topic string) {
	savedCb = nil
}

func (*pubSubDatasource) Publish(topic string, jsonData []byte, source string) {
	if topic == broadcastRequestTopic {
		return
	}

	cb := savedCb
	if cb != nil {
		data := map[string][]mcv1.PeerService{}
		err := json.Unmarshal(jsonData, &data)
		if err != nil {
			return
		}
		go func() {
			time.Sleep(5 * time.Second)
			publishImpersonating(data[source], "other-cluster-2")
			publishImpersonating(data[source], "other-cluster-3")
		}()
	}
}

func publishImpersonating(ps []mcv1.PeerService, name string) {
	jsonData, err := json.Marshal(map[string][]mcv1.PeerService{name: cloneWithClusterName(ps, name)})
	if err != nil {
		log.Printf("Error marshalling peer service list: %v", err)
		return
	}
	cb := savedCb
	if cb == nil {
		log.Printf("No callback registered")
		return
	}
	cb(jsonData, name)
}

// New creates a fresh PubSub datasource
func New(_ Provider) ExternalSource {
	return &pubSubDatasource{}
}

func cloneWithClusterName(ps []mcv1.PeerService, name string) []mcv1.PeerService {
	var cpy = make([]mcv1.PeerService, len(ps))
	for i := range ps {
		cpy[i] = mcv1.PeerService{
			Cluster:     name,
			ServiceName: ps[i].ServiceName,
			Endpoints:   ps[i].Endpoints,
			Ports:       ps[i].Ports,
		}
	}
	return cpy
}
