package datasource

// ExternalSource represents the datasource for all ServiceSync notifications
type ExternalSource interface {
	Subscribe(topic string, cb func(jsonData []byte, source string))
	Unsubscribe(topic string)
	Publish(topic string, jsonData []byte, source string)
}
