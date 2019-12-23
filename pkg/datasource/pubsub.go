// +build !mock

// Package datasource defines here a the PubSub datasource for cloud operation
package datasource

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gocloud.dev/pubsub"
)

type callback = func(jsonData []byte, source string)

// TopicSettings defines the strings that need to be send to the pubsub implementation
type TopicSettings interface {
	TopicURL() string
	SubscriptionURL() string
}

type pubSubDatasource struct {
	topics                        map[string]*pubsub.Topic
	subscriptionTasks             map[string]chan callback
	parseTopic                    func(url string) TopicSettings
	ensurePubSubTopicSubscription func(settings TopicSettings) error
}

var _ ExternalSource = &pubSubDatasource{}
var ctx = context.Background()

func (p *pubSubDatasource) Subscribe(url string, cb func(jsonData []byte, source string)) {
	setting := p.parseTopic(url)
	sub := p.getSubscription(setting)
	sub <- cb
}

func (p *pubSubDatasource) Unsubscribe(url string) {
	setting := p.parseTopic(url)
	if currentTask, hasTask := p.subscriptionTasks[setting.TopicURL()]; hasTask {
		currentTask <- nil
		return
	}
	delete(p.subscriptionTasks, setting.TopicURL())
}

func (p *pubSubDatasource) Publish(url string, jsonData []byte, source string) {
	setting := p.parseTopic(url)
	top := p.getTopic(setting)
send:
	err := top.Send(ctx, &pubsub.Message{
		Body:     jsonData,
		Metadata: map[string]string{"sender": source},
	})
	if err != nil {
		log.Printf("Error while publishing to %s: %v", setting.TopicURL(), err)
		if strings.Contains(fmt.Sprint(err), "NotFound") {
			err = p.ensurePubSubTopicSubscription(setting)
			if err == nil {
				time.Sleep(1 * time.Second)
				goto send
			}
			log.Printf("Error while ensuring topic: %s", err)
			panic(err)
		}
	}
}

func (p *pubSubDatasource) getTopic(setting TopicSettings) *pubsub.Topic {
	// Return cached topic
	if topic, hasTopic := p.topics[setting.TopicURL()]; hasTopic {
		return topic
	}

	// Open topic
open:
	top, err := pubsub.OpenTopic(ctx, setting.TopicURL())
	if err != nil {
		log.Printf("Error while opening topic: %s", err)
		err = p.ensurePubSubTopicSubscription(setting)
		if err == nil {
			time.Sleep(1 * time.Second)
			goto open
		}
		log.Printf("Error while ensuring topic: %s", err)
		panic(err)
	}
	p.topics[setting.TopicURL()] = top
	return top
}

func (p *pubSubDatasource) getSubscription(setting TopicSettings) chan callback {
	// Subscribe only once: send existing
	if currentTask, hasTask := p.subscriptionTasks[setting.TopicURL()]; hasTask {
		return currentTask
	}

	// Setup goroutines & register callback channel
	currentTask := make(chan callback, 1)
	messages := make(chan *pubsub.Message, 0)
	p.subscriptionTasks[setting.TopicURL()] = currentTask

	// Goroutine that receives pubsub messages
	go func() {
		// Open subscription
	open:
		sub, err := pubsub.OpenSubscription(ctx, setting.SubscriptionURL())
		if err != nil {
			log.Printf("Error while opening subscription: %s", err)
			err = p.recover(setting)
			if err == nil {
				goto open
			}
			log.Printf("Error while ensuring subscription: %s", err)
			panic(err)
		}
		// Loop
		for {
			msg, err := sub.Receive(ctx)
			if err != nil {
				log.Printf("Error in subscription: %v", err)
				err = p.recover(setting)
				if err == nil {
					goto open
				}
				continue
			}
			messages <- msg
		}
	}()

	// Goroutine that delivers pubsub messages
	go func() {
		var cb callback
		for {
			select {
			case newCb := <-currentTask:
				if newCb == nil {
					return
				}
				cb = newCb
			case msg := <-messages:
				cb(msg.Body, msg.Metadata["sender"])
				msg.Ack()
			}
		}
	}()

	return currentTask
}

func (p *pubSubDatasource) recover(setting TopicSettings) error {
	err := p.ensurePubSubTopicSubscription(setting)
	if err == nil {
		return nil
	}
	time.Sleep(1 * time.Second)
	return err
}

// New creates a fresh PubSub datasource
func New(provider Provider) ExternalSource {
	return &pubSubDatasource{
		topics:                        make(map[string]*pubsub.Topic, 0),
		subscriptionTasks:             make(map[string]chan callback, 0),
		parseTopic:                    provider.ParseTopic,
		ensurePubSubTopicSubscription: provider.EnsurePubSubTopicSubscription,
	}
}
