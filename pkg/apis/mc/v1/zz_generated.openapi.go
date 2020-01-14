// +build !ignore_autogenerated

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1

import (
	spec "github.com/go-openapi/spec"
	common "k8s.io/kube-openapi/pkg/common"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"./pkg/apis/mc/v1.Cluster":           schema_pkg_apis_mc_v1_Cluster(ref),
		"./pkg/apis/mc/v1.PeerEndpoint":      schema_pkg_apis_mc_v1_PeerEndpoint(ref),
		"./pkg/apis/mc/v1.PeerPort":          schema_pkg_apis_mc_v1_PeerPort(ref),
		"./pkg/apis/mc/v1.PeerService":       schema_pkg_apis_mc_v1_PeerService(ref),
		"./pkg/apis/mc/v1.ServiceSync":       schema_pkg_apis_mc_v1_ServiceSync(ref),
		"./pkg/apis/mc/v1.ServiceSyncSpec":   schema_pkg_apis_mc_v1_ServiceSyncSpec(ref),
		"./pkg/apis/mc/v1.ServiceSyncStatus": schema_pkg_apis_mc_v1_ServiceSyncStatus(ref),
	}
}

func schema_pkg_apis_mc_v1_Cluster(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "Cluster represents a set of parameters of a cluster",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"name": {
						SchemaProps: spec.SchemaProps{
							Description: "Which clusters are we receiving data from?",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"services": {
						SchemaProps: spec.SchemaProps{
							Description: "Which endpoints did we receive from those clusters?",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: ref("./pkg/apis/mc/v1.PeerService"),
									},
								},
							},
						},
					},
					"lastUpdate": {
						SchemaProps: spec.SchemaProps{
							Description: "Last time the data was received (when remote) or published (when local)",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Time"),
						},
					},
				},
				Required: []string{"name"},
			},
		},
		Dependencies: []string{
			"./pkg/apis/mc/v1.PeerService", "k8s.io/apimachinery/pkg/apis/meta/v1.Time"},
	}
}

func schema_pkg_apis_mc_v1_PeerEndpoint(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PeerEndpoint represents a Node from a Service in a remote cluster",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"ipAddress": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
					"hostname": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
				},
				Required: []string{"ipAddress", "hostname"},
			},
		},
	}
}

func schema_pkg_apis_mc_v1_PeerPort(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PeerPort represents a port of a Service in a remote cluster",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"internalPort": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"integer"},
							Format: "int32",
						},
					},
					"externalPort": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"integer"},
							Format: "int32",
						},
					},
				},
				Required: []string{"internalPort", "externalPort"},
			},
		},
	}
}

func schema_pkg_apis_mc_v1_PeerService(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "PeerService represents a Service in a remote cluster",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"cluster": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
					"serviceName": {
						SchemaProps: spec.SchemaProps{
							Type:   []string{"string"},
							Format: "",
						},
					},
					"endpoints": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-list-type": "set",
							},
						},
						SchemaProps: spec.SchemaProps{
							Type: []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: ref("./pkg/apis/mc/v1.PeerEndpoint"),
									},
								},
							},
						},
					},
					"ports": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-list-type": "set",
							},
						},
						SchemaProps: spec.SchemaProps{
							Type: []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: ref("./pkg/apis/mc/v1.PeerPort"),
									},
								},
							},
						},
					},
				},
				Required: []string{"cluster", "serviceName", "endpoints", "ports"},
			},
		},
		Dependencies: []string{
			"./pkg/apis/mc/v1.PeerEndpoint", "./pkg/apis/mc/v1.PeerPort"},
	}
}

func schema_pkg_apis_mc_v1_ServiceSync(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ServiceSync is the Schema for the servicesyncs API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/mc/v1.ServiceSyncSpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Ref: ref("./pkg/apis/mc/v1.ServiceSyncStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"./pkg/apis/mc/v1.ServiceSyncSpec", "./pkg/apis/mc/v1.ServiceSyncStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_pkg_apis_mc_v1_ServiceSyncSpec(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ServiceSyncSpec defines the desired state of ServiceSync",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"selector": {
						SchemaProps: spec.SchemaProps{
							Description: "Label selector for services. Only the services matching this selector will be published.",
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.LabelSelector"),
						},
					},
					"topicURL": {
						SchemaProps: spec.SchemaProps{
							Description: "URL of the PubSub topic, specified as for example \"gcppubsub://projects/myproject/topics/mytopic\".",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"reconcileInterval": {
						SchemaProps: spec.SchemaProps{
							Description: "Publish & update interval in seconds, default 300 s = 5 minutes. Golang duration string.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"prunePeerAtAge": {
						SchemaProps: spec.SchemaProps{
							Description: "Age in seconds before removing a peer from the remote services. Default is 0, which means never. Golang duration string. If used, make sure to align the ReconcileInterval & PruneRemoteAtAge between clusters while keeping safe margins.",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"endpointsPublishPreferLoadBalancerIPs": {
						SchemaProps: spec.SchemaProps{
							Description: "Whether Load Balancer IPs must be published instead of node ips if those are configured by the provider platform.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"endpointsUseExternalIPs": {
						SchemaProps: spec.SchemaProps{
							Description: "Whether to use the external IP addresses of nodes, or use the internal IPs (the default). Use internal IPs if the clusters share a network.",
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"endpointsPublishMax": {
						SchemaProps: spec.SchemaProps{
							Description: "How many endpoints to publish from this cluster (e.g. how many nodes should act as entry point). 0 is unlimited. Set this to a lower value if this cluster has a lot of nodes, and the amount of data to sync becomes prohibitive. Note that the limited set of nodes must be capable enough to accept the traffic and must be highly available, e.g. setting it to 1 is not advisable.",
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
					"endpointsConfigureMax": {
						SchemaProps: spec.SchemaProps{
							Description: "How many endpoints to configure for each service in this cluster (e.g. how many nodes should act as entry point). 0 is unlimited. Set this to a lower value if any of the clusters has a lot of nodes and a lot of endpoints from this to that cluster causes troubles with the amount of ip table rules.",
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
				},
				Required: []string{"selector", "topicURL"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.LabelSelector"},
	}
}

func schema_pkg_apis_mc_v1_ServiceSyncStatus(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "ServiceSyncStatus defines the observed state of ServiceSync",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"clusters": {
						SchemaProps: spec.SchemaProps{
							Description: "Data of all the clusters (including self)",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Ref: ref("./pkg/apis/mc/v1.Cluster"),
									},
								},
							},
						},
					},
					"peers": {
						VendorExtensible: spec.VendorExtensible{
							Extensions: spec.Extensions{
								"x-kubernetes-list-type": "set",
							},
						},
						SchemaProps: spec.SchemaProps{
							Description: "Which peers are available",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Type:   []string{"string"},
										Format: "",
									},
								},
							},
						},
					},
				},
			},
		},
		Dependencies: []string{
			"./pkg/apis/mc/v1.Cluster"},
	}
}
