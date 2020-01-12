package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceSyncSpec defines the desired state of ServiceSync
// +k8s:openapi-gen=true
type ServiceSyncSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Label selector for services. Only the services matching this selector will be published.
	Selector metav1.LabelSelector `json:"selector"`

	// URL of the PubSub topic, specified as for example "gcppubsub://projects/myproject/topics/mytopic".
	TopicURL string `json:"topicURL"`

	// Whether Load Balancer IPs must be published instead of node ips if those are configured by the provider platform.
	EndpointsPublishPreferLoadBalancerIPs *bool `json:"endpointsPublishPreferLoadBalancerIPs,omitempty"`

	// How many endpoints to publish from this cluster (e.g. how many nodes should act as entry point).
	// 0 is unlimited. Set this to a lower value if this cluster has a lot of nodes, and the amount of data to sync becomes prohibitive.
	// Note that the limited set of nodes must be capable enough to accept the traffic and must be highly available, e.g. setting it to 1 is not advisable.
	EndpointsPublishMax *int32 `json:"endpointsPublishMax,omitempty"`

	// How many endpoints to configure for each service in this cluster (e.g. how many nodes should act as entry point).
	// 0 is unlimited. Set this to a lower value if any of the clusters has a lot of nodes and a lot of endpoints from this to that cluster causes troubles with the amount of ip table rules.
	EndpointsConfigureMax *int32 `json:"endpointsConfigureMax,omitempty"`
}

// ServiceSyncStatus defines the observed state of ServiceSync
// +k8s:openapi-gen=true
type ServiceSyncStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// Data of all the clusters (including self)
	// +listType=map
	// +listMapKey=name
	// +optional
	Clusters []Cluster `json:"clusters,omitempty"`
}

// Cluster represents a set of parameters of a cluster
// +k8s:openapi-gen=true
type Cluster struct {
	// Which clusters are we receiving data from?
	Name string `json:"name"`
	// Which endpoints did we receive from those clusters?
	// +listType=map
	// +listMapKey=serviceName
	Services []PeerService `json:"services,omitempty"`
	// Last time the data was received (when remote) or published (when local)
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
}

// PeerService represents a Service in a remote cluster
// +k8s:openapi-gen=true
type PeerService struct {
	Cluster     string `json:"cluster"`
	ServiceName string `json:"serviceName"`
	// +listType=set
	Endpoints []PeerEndpoint `json:"endpoints"`
	// +listType=set
	Ports []PeerPort `json:"ports"`
}

// PeerEndpoint represents a Node from a Service in a remote cluster
// +k8s:openapi-gen=true
type PeerEndpoint struct {
	IPAddress string `json:"ipAddress"`
	Hostname  string `json:"hostname"`
}

// PeerPort represents a port of a Service in a remote cluster
// +k8s:openapi-gen=true
type PeerPort struct {
	InternalPort int32 `json:"internalPort"`
	ExternalPort int32 `json:"externalPort"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceSync is the Schema for the servicesyncs API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=servicesyncs,scope=Namespaced
type ServiceSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSyncSpec   `json:"spec,omitempty"`
	Status ServiceSyncStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceSyncList contains a list of ServiceSync
type ServiceSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceSync{}, &ServiceSyncList{})
}
