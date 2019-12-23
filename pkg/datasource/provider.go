package datasource

// Provider represents the provider-specific functionality that go.cloud.dev lacks
type Provider struct {
	ParseTopic                    func(url string) TopicSettings
	EnsurePubSubTopicSubscription func(setting TopicSettings) error
}
