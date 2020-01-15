// Package datasourcegoogle defines here a the PubSub datasource for cloud operation
package datasourcegoogle

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"q42/mc-robot/pkg/datasource"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	pubsub "cloud.google.com/go/pubsub"
	// Include Google PubSub driver for PubSub
	_ "gocloud.dev/pubsub/gcppubsub"
)

var log = logf.Log.WithName("pubsub")
var ctx = context.Background()
var clients map[string]*pubsub.Client = make(map[string]*pubsub.Client, 0)

var urlSyntax = regexp.MustCompile("^gcppubsub://projects/(.+)/(topics|subscriptions)/(.+)$")
var alphaNum = regexp.MustCompile("[^a-z0-9-]")

type topicSettings struct {
	project         string
	topicURL        string
	topicID         string
	subscriptionURL string
	subscriptionID  string
}

func (s *topicSettings) SubscriptionURL() string { return s.subscriptionURL }
func (s *topicSettings) TopicURL() string        { return s.topicURL }

// Google is a PubSub provider implementing our Datasource
var Google = datasource.Provider{
	ParseTopic:                    parseTopic,
	EnsurePubSubTopicSubscription: ensurePubSubTopicSubscription,
}

func parseTopic(topic string) datasource.TopicSettings {
	match := urlSyntax.FindStringSubmatch(topic)
	if len(match) < 2 {
		panic(fmt.Sprintf("Invalid topic ('%s'): format must be gcppubsub://projects/(.+)/(topics|subscriptions)/(.+)", topic))
	}

	// Unique subscription url
	hostname, _ := os.Hostname()
	hostname = alphaNum.ReplaceAllString(hostname, "-")
	uniqueID := firstNonEmpty(os.Getenv("PUBSUB_UNIQUE_CLIENTID"), hostname)
	subID := fmt.Sprintf("%s-%s", match[3], uniqueID)
	topURL := fmt.Sprintf("gcppubsub://projects/%s/topics/%s", match[1], match[3])
	subURL := fmt.Sprintf("gcppubsub://projects/%s/subscriptions/%s", match[1], subID)

	return datasource.TopicSettings(&topicSettings{
		project:         match[1],
		topicURL:        topURL,
		topicID:         match[3],
		subscriptionURL: subURL,
		subscriptionID:  subID,
	})
}

func ensurePubSubTopicSubscription(setting datasource.TopicSettings) error {
	client := getClient((setting.(*topicSettings)).project)
	topicID := setting.(*topicSettings).topicID
	subscriptionID := setting.(*topicSettings).subscriptionID

	// Check if topic exists or create it
	exists, err := client.Topic(topicID).Exists(ctx)
	if err != nil {
		log.Printf("Error checking if topic exists %s", err)
		return err
	}
	if !exists {
		log.Printf("Creating topic %s", topicID)
		_, err := client.CreateTopic(ctx, topicID)
		if err != nil {
			return err
		}
	}

	// Check if subscription exists or create it
	exists, err = client.Subscription(subscriptionID).Exists(ctx)
	if err != nil {
		log.Printf("Error checking if subscription exists %s", err)
		return err
	}
	if !exists {
		log.Printf("Creating subscription %s", subscriptionID)
		_, err := client.CreateSubscription(ctx, subscriptionID, pubsub.SubscriptionConfig{
			Topic:            client.Topic(topicID),
			AckDeadline:      20 * time.Second,
			ExpirationPolicy: 25 * time.Hour,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func getClient(project string) *pubsub.Client {
	if existing, hasClient := clients[project]; hasClient {
		return existing
	}
	client, err := pubsub.NewClient(context.Background(), project)
	if err != nil {
		panic(err)
	}
	clients[project] = client
	return client
}

func firstNonEmpty(strings ...string) string {
	for _, str := range strings {
		if str != "" {
			return str
		}
	}
	return ""
}
