package servicesync

import (
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"context"
	stderrors "errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReconcileServiceSync) getLocalNodeList() (map[string]bool, error) {
	nodes := &corev1.NodeList{}
	err := r.client.List(context.Background(), nodes)
	if err != nil {
		log.Error(err, "Error while reading NodeList")
		return nil, err
	}

	schedulableMap := make(map[string]bool, len(nodes.Items))
	for _, node := range nodes.Items {
		schedulableMap[node.ObjectMeta.Name] = !node.Spec.Unschedulable
	}
	return schedulableMap, nil
}

func (r *ReconcileServiceSync) getLocalServiceMap(sync *mcv1.ServiceSync) ([]mcv1.PeerService, error) {
	log.Info(fmt.Sprintf("Computing ServiceMap for %s", sync.Name))
	services := &corev1.ServiceList{}
	selector, err := metav1.LabelSelectorAsSelector(&sync.Spec.Selector)
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return nil, err
	}
	err = r.client.List(context.Background(), services, client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return nil, err
	}

	nodes, err := r.getNodes()
	clusterName = r.getClusterName()
	// log.Info(fmt.Sprintf("Node %#v", nodes.Items[0]))
	peerServices := make([]mcv1.PeerService, 0)
	for _, service := range services.Items {
		ports := make([]mcv1.PeerPort, 0)
		for _, port := range service.Spec.Ports {
			if port.NodePort > 0 {
				ports = append(ports, mcv1.PeerPort{
					InternalPort: port.Port,
					ExternalPort: port.NodePort,
				})
			}
		}
		if len(ports) > 0 {
			peerServices = append(peerServices, mcv1.PeerService{
				Cluster:     clusterName,
				ServiceName: service.Name,
				Endpoints:   endpointsForHostsAndPort(nodes),
				Ports:       ports,
			})
		}
	}
	return peerServices, nil
}

func (r *ReconcileServiceSync) getNodes() ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := r.client.List(context.Background(), nodes)
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return nil, err
	}
	if len(nodes.Items) == 0 {
		err = stderrors.New("No nodes found")
		log.Error(err, "Error while reading NodeList")
		return nil, err
	}
	return nodes.Items, nil
}

func (r *ReconcileServiceSync) getServiceSyncs() ([]mcv1.ServiceSync, error) {
	syncs := &mcv1.ServiceSyncList{}
	err := r.client.List(context.TODO(), syncs)
	if err != nil {
		log.Error(err, "Error while reading ServiceSyncs")
		return nil, err
	}
	return syncs.Items, nil
}
