// +build !mock

// Package datasource defines here a the PubSub datasource for cloud operation
package datasource

import (
	"context"
	"fmt"
	"log"
	"os"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"

	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/gcppubsub"
)

var project = os.Getenv("PROJECT")

type datasourceSub struct {
	Sub *pubsub.Subscription
	Cb  func(jsonData []byte, source string)
}

type datasourcePublish struct {
	Data     []byte
	Metadata map[string]string
}

var topics = make(map[string]chan datasourcePublish, 0)
var topicSubscriptions = make(map[string]datasourceSub, 0)

const broadcastRequestTopic = "broadcastRequest"

type pubSubDatasource struct {
}

var _ ExternalSource = &pubSubDatasource{}

func (p *pubSubDatasource) Subscribe(topic string, cb func(jsonData []byte, source string)) {
	topic = fmt.Sprintf("gcppubsub://projects/%s/topics/%s", project, topic)
	p.subscribe(topic, cb)
}

func (*pubSubDatasource) subscribe(topic string, cb func(jsonData []byte, source string)) {
	ctx := context.Background()
	sub, hasSubscription := topicSubscriptions[topic]
	if hasSubscription {
		topicSubscriptions[topic] = datasourceSub{
			Sub: sub.Sub,
			Cb:  cb,
		}
	} else {
		sub, err := pubsub.OpenSubscription(ctx, topic)
		if err != nil {
			log.Print(err)
			return
		}
		topicSubscriptions[topic] = datasourceSub{Sub: sub, Cb: cb}
		go func() error {
			top, err := pubsub.OpenSubscription(ctx, topic)
			if err != nil {
				return err
			}
			for {
				msg, err := top.Receive(ctx)
				topicSubscriptions[topic].Cb(msg.Body, msg.Metadata["sender"])
				if err != nil {
					log.Printf("Error in subscription: %v", err)
					break
				}
			}
			defer top.Shutdown(ctx)
			return nil
		}()
	}
}

func (p *pubSubDatasource) Unsubscribe(topic string) {
	topic = fmt.Sprintf("gcppubsub://projects/%s/topics/%s", project, topic)
	p.unsubscribe(topic)
}

func (*pubSubDatasource) unsubscribe(topic string) {
	sub, hasSubscription := topicSubscriptions[topic]
	if hasSubscription {
		sub.Sub.Shutdown(context.Background())
	}
	delete(topicSubscriptions, topic)
}

func (p *pubSubDatasource) Publish(topic string, jsonData []byte, source string) {
	topic = fmt.Sprintf("gcppubsub://projects/%s/subscriptions/%s", project, topic)
	p.publish(topic, jsonData, source)
}

func (*pubSubDatasource) publish(topic string, jsonData []byte, source string) {
	if topic == broadcastRequestTopic {
		return
	}

	ctx := context.Background()
	dest, hasTopic := topics[topic]
	if !hasTopic {
		dest = make(chan datasourcePublish, 0)
		topics[topic] = dest
		go func() {
			top, err := pubsub.OpenTopic(ctx, topic)
			if err != nil {
				log.Print(err)
				return
			}
			for {
				msg := <-dest
				err := top.Send(ctx, &pubsub.Message{
					Body:     msg.Data,
					Metadata: msg.Metadata,
				})
				if err != nil {
					break
				}
			}
			defer top.Shutdown(ctx)
		}()
	}

	dest <- datasourcePublish{Data: jsonData, Metadata: map[string]string{"sender": source}}
}

// New creates a fresh PubSub datasource
func New() ExternalSource {
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
