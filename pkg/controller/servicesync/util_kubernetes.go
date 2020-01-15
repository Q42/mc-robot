package servicesync

import (
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"context"
	stderrors "errors"

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

func (r *ReconcileServiceSync) getLocalServiceMap(sync *mcv1.ServiceSync) (map[string]*mcv1.PeerService, error) {
	logger := log.WithValues("servicesync", sync.Name)
	logger.Info("Computing ServiceMap")

	// Default is internal ips
	useExternalIP := sync.Spec.EndpointsUseExternalIPs != nil && *sync.Spec.EndpointsUseExternalIPs == true

	services := &corev1.ServiceList{}
	selector, err := metav1.LabelSelectorAsSelector(&sync.Spec.Selector)
	if err != nil {
		logger.Error(err, "Error while computing ServiceMap")
		return nil, err
	}
	err = r.client.List(context.Background(), services, client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		logger.Error(err, "Error while computing ServiceMap")
		return nil, err
	}

	nodes, err := r.getNodes()
	clusterName = r.getClusterName()

	peerServices := make(map[string]*mcv1.PeerService, 0)
	for _, service := range services.Items {
		ports := make([]mcv1.PeerPort, 0)
		lbPorts := make([]mcv1.PeerPort, 0)
		for _, port := range service.Spec.Ports {
			if port.NodePort > 0 {
				ports = append(ports, mcv1.PeerPort{
					InternalPort: port.Port,
					ExternalPort: port.NodePort,
				})
				lbPorts = append(ports, mcv1.PeerPort{
					InternalPort: port.Port,
					ExternalPort: port.Port, // load balancers expose the cluster-internal port
				})
			}
		}

		// Load-Balancer service
		if len(ports) > 0 && len(service.Status.LoadBalancer.Ingress) > 0 && shouldPublishLB(sync) {
			peerServices[service.Name] = &mcv1.PeerService{
				Cluster:     clusterName,
				ServiceName: service.Name,
				Endpoints:   endpointsForIngresses(service.Status.LoadBalancer.Ingress),
				Ports:       lbPorts,
			}
		} else
		// Regular ClusterIP/NodePort (non-loadbalancer) service
		if len(ports) > 0 {
			peerServices[service.Name] = &mcv1.PeerService{
				Cluster:     clusterName,
				ServiceName: service.Name,
				Endpoints:   endpointsForHostsAndPort(nodes, useExternalIP),
				Ports:       ports,
			}
		} else {
			logger.Info("Skipping service with only non-NodePort ports", "service", service.Name)
		}
	}
	return peerServices, nil
}

func (r *ReconcileServiceSync) getNodes() ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := r.client.List(context.Background(), nodes)
	if err == nil && len(nodes.Items) == 0 {
		err = stderrors.New("No nodes found")
	}
	if err != nil {
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

func shouldPublishLB(sync *mcv1.ServiceSync) bool {
	setting := sync.Spec.EndpointsPublishPreferLoadBalancerIPs
	return setting != nil && *setting == true
}
