package servicesync

import (
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	clusterNameLabel = "ClusterName" // clusterv1.ClusterNameLabel (from "sigs.k8s.io/cluster-api/api/v1alpha3" = master)
	gkeNodePoolLabel = "cloud.google.com/gke-nodepool"
)

// getClusterName retrieves the name of the cluster by sampling the nodes
// if the CLUSTER_NAME environment variable is set, it will use that instead
func (r *ReconcileServiceSync) getClusterName() string {
	if clusterName != "" {
		return clusterName
	}

	if os.Getenv("CLUSTER_NAME") != "" {
		clusterName = os.Getenv("CLUSTER_NAME")
		return clusterName
	}

	nodes, err := r.getNodes()
	logOnError(err, "Failed to get nodes for getClusterName")
	clusterName = getClusterName(nodes)
	return clusterName
}

// getClusterName retrieves the name of the cluster by sampling the nodes
// if the CLUSTER_NAME environment variable is set, it will use that instead
func getClusterName(nodes []corev1.Node) string {
	if os.Getenv("CLUSTER_NAME") != "" {
		clusterName = os.Getenv("CLUSTER_NAME")
		return clusterName
	}

	if len(nodes) == 0 {
		return ""
	}

	node := nodes[0]
	if node.Labels[clusterNameLabel] != "" {
		return node.Labels[clusterNameLabel]
	}
	if node.ClusterName != "" {
		return node.ClusterName
	}

	// Hack for clusters that don't have ClusterName as a label on the nodes (pre-1.15?)
	if _, hasLabel := node.Labels[gkeNodePoolLabel]; hasLabel {
		// Split/TrimPrefix:
		// gke-mycluster-1-node-pool-1-b486c6b7-chm7
		// pre^clusterName^postfix_____________^node-hash
		prefix := "gke-"
		postfix := "-" + node.Labels[gkeNodePoolLabel]
		clusterName := strings.Split(strings.TrimPrefix(node.Name, prefix), postfix)[0]
		log.Info(fmt.Sprintf("getClusterName: used a hack to determine the clusterName from hostname %s", node.Name))
		return clusterName
	}
	log.Info(fmt.Sprintf("getClusterName from %#v", node))
	panic("ClusterName could not be determined")
}
