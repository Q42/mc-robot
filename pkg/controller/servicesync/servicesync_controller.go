package servicesync

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"q42/mc-robot/pkg/datasource"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName   = "mc_robot"
	clusterNameLabel = "ClusterName" // clusterv1.ClusterNameLabel (from "sigs.k8s.io/cluster-api/api/v1alpha3" = master)
	gkeNodePoolLabel = "cloud.google.com/gke-nodepool"
	eventTypeNormal  = "Normal"  // kubernetes supports values Normal & Warning
	eventTypeWarning = "Warning" // kubernetes supports values Normal & Warning
)

var log = logf.Log.WithName("controller_servicesync")
var remoteStatus = make(map[string][]mcv1.PeerService, 0)
var pubSubTriggers = make(chan reconcile.Request, 0)
var clusterName string

// Add creates a new ServiceSync Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, es datasource.ExternalSource) error {
	return add(mgr, newReconciler(mgr, es))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, es datasource.ExternalSource) reconcile.Reconciler {
	return &ReconcileServiceSync{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
		es:       es,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	reconciler := r.(*ReconcileServiceSync)

	// Create a new controller
	c, err := controller.New("servicesync-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ServiceSync
	err = c.Watch(&source.Kind{Type: &mcv1.ServiceSync{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { log.Info("Create event"); return true },
		DeleteFunc:  func(e event.DeleteEvent) bool { log.Info("Delete event"); return true },
		UpdateFunc:  func(e event.UpdateEvent) bool { log.Info("Update event"); return true },
		GenericFunc: func(e event.GenericEvent) bool { log.Info("Generic event"); return true },
	})
	if err != nil {
		return err
	}

	// Watch endpoints that we are the owner of (for if endpoints would be deleted manually, we'll recreate them)
	err = c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, &handler.EnqueueRequestForOwner{IsController: true, OwnerType: &mcv1.ServiceSync{}})
	if err != nil {
		return err
	}

	// Watch services:
	// - those pointing to our cluster (those which we might publish) &
	// - those pointing to other clusters that we configured and which we must keep up to date.
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			service, ok := a.Object.(*corev1.Service)
			if !ok {
				log.Info("Uncastable corev1.Service")
				return []reconcile.Request{}
			}

			requests := []reconcile.Request{}
			syncs, err := reconciler.getServiceSyncs()
			if err != nil {
				return []reconcile.Request{}
			}

			// For any ServiceSync which Selector matches this changed service, enqueue a request
			for _, sync := range syncs {
				selector, err := metav1.LabelSelectorAsSelector(&sync.Spec.Selector)
				if err == nil && selector.Matches(labels.Set(service.Labels)) {
					log.Info(fmt.Sprintf("Service '%s' is elegible for ServiceSync '%s'", service.Name, sync.Name))
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
						Name:      sync.GetName(),
						Namespace: sync.GetNamespace(),
					}})
				}
			}

			// For any Service that a ServiceSync is the owner of, enqueue the owner ServiceSync
			if ownerRef := metav1.GetControllerOf(a.Meta); ownerRef != nil {
				log.Info(fmt.Sprintf("Service OwnerRef %#v", ownerRef))
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      ownerRef.Name,
					Namespace: a.Meta.GetNamespace(),
				}})
			}

			return requests
		}),
	})
	if err != nil {
		return err
	}

	// Watch nodes, because we must publish this information
	var nodeList map[string]bool
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
			// Initially: lookup all nodes
			if nodeList == nil {
				nodeList, err = reconciler.getLocalNodeList()
				if err != nil {
					log.Error(err, "Error while fetching node list")
					return []reconcile.Request{}
				}
				log.Info("Filling initial node list & enqueuing sync.")
				return reconciler.enqueueAllServiceSyncs(a)
			}
			// Check if something changed
			if node, ok := a.Object.(*corev1.Node); ok && node.Spec.Unschedulable == nodeList[node.ObjectMeta.Name] {
				nodeList[node.ObjectMeta.Name] = !node.Spec.Unschedulable
				log.Info("Node schedulability changed. Propagating.")
				return reconciler.enqueueAllServiceSyncs(a)
			}
			// Else do nothing
			return []reconcile.Request{}
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileServiceSync implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileServiceSync{}

// ReconcileServiceSync reconciles a ServiceSync object
type ReconcileServiceSync struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	es       datasource.ExternalSource
}

// Reconcile reads that state of the cluster for a ServiceSync object and makes changes based on the state read
// and what is in the ServiceSync.Spec
func (r *ReconcileServiceSync) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ServiceSync")
	ctx := context.Background()

	// Fetch the ServiceSync instance
	instance := &mcv1.ServiceSync{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request. Return and don't requeue.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "failure getting ServiceSync")
		return reconcile.Result{}, err
	}

	r.es.Subscribe(request.NamespacedName.String(), r.callbackFor(request.NamespacedName))
	err = r.ensurePeerServices(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	sm, _ := r.getLocalServiceMap(instance)
	if clusterName == "" {
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, err
	}

	if instance.Status.LastPublishTime.IsZero() || instance.Status.LastPublishTime.Add(5*time.Minute).Before(time.Now()) {
		// published too long ago (or never), so publish!
		jsonData, err := json.Marshal(map[string][]mcv1.PeerService{clusterName: sm})
		if err != nil {
			return reconcile.Result{}, err
		}
		r.es.Publish(request.NamespacedName.String(), jsonData, clusterName)
		// then reschedule reconcile after 5 minutes again
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	return reconcile.Result{}, nil
}

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
	clusterName = getClusterName(nodes)
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

// Build according to example custom EnqueueRequestsFromMapFunc:
// https://github.com/kubernetes-sigs/controller-runtime/blob/dfc2508132/pkg/handler/example_test.go#L69-L78
func (r *ReconcileServiceSync) enqueueAllServiceSyncs(a handler.MapObject) []reconcile.Request {
	syncs := &mcv1.ServiceSyncList{}
	err := r.client.List(context.TODO(), syncs)
	if err != nil {
		log.Error(err, "Error while responding to node event")
		return []reconcile.Request{}
	}

	reqs := []reconcile.Request{}
	for _, sync := range syncs.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      sync.GetName(),
			Namespace: sync.GetNamespace(),
		}})
	}
	return reqs
}

func (r *ReconcileServiceSync) callbackFor(name types.NamespacedName) func([]byte, string) {
	return func(dataJson []byte, from string) {
		// Unmarshal data
		freshData := map[string][]mcv1.PeerService{}
		err := json.Unmarshal(dataJson, &freshData)
		if err != nil {
			return
		}

		// Merge fresh data into previous state
		data := map[string][]mcv1.PeerService{}
		instance := &mcv1.ServiceSync{}
		err = r.client.Get(context.Background(), name, instance)
		data = instance.Status.PeerServices
		for _, cluster := range append(keys(data), keys(freshData)...) {
			if services, hasCluster := freshData[cluster]; hasCluster {
				data[cluster] = services
			}
		}

		// Save
		err = r.ensureRemoteStatus(name, data)
		if err != nil {
			r.recorder.Eventf(instance, eventTypeWarning, "FailureEnsuring", fmt.Sprintf("Failed to ensure remote state: %v", err))
		} else {
			r.recorder.Eventf(instance, eventTypeNormal, "PubSubMessage", "Remote status update received: need to reconcile local endpoints.")
		}
	}
}

func (r *ReconcileServiceSync) ensureRemoteStatus(name types.NamespacedName, status map[string][]mcv1.PeerService) error {
	ctx := context.Background()
	instance := &mcv1.ServiceSync{}

	// Load latest state
	err := r.client.Get(ctx, name, instance)
	if err != nil {
		return err
	}
	oldStatus := instance.Status.DeepCopy()

	// Modify
	instance.Status.SelectedServices = orElse(instance.Status.SelectedServices, make([]string, 0)).([]string)
	instance.Status.PeerClusters = keys(status)
	instance.Status.PeerServices = status
	// Test: is this necessary? instance.Status.LastPublishTime = metav1.NewTime(time.Now())

	// Patch if necessary
	if !operatorStatusesEqual(*oldStatus, instance.Status) {
		// update the Status of the resource with the special client.Status()-client (nothing happens when you don't use the sub-client):
		err = r.client.Status().Update(ctx, instance)
		if err == nil {
			log.Info(fmt.Sprintf("Patched status of %s", name))
		} else {
			log.Info(fmt.Sprintf("Patching status of %s failed: %v", name, err))
		}
		return err
	}
	log.Info("Status identical, nothing to do")
	return nil
}

func (r *ReconcileServiceSync) ensurePeerServices(instance *mcv1.ServiceSync) error {
	existingServices := &corev1.ServiceList{}
	err := r.client.List(context.Background(), existingServices)
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return err
	}

	for _, services := range instance.Status.PeerServices {
		for _, service := range services {
			if len(service.Ports) == 0 {
				continue
			}

			desiredService, desiredEndpoints := serviceForPeer(service, instance.GetNamespace())
			currentService := &corev1.Service{}
			currentEndpoints := &corev1.Endpoints{}
			err := r.client.Get(context.Background(), types.NamespacedName{Name: desiredService.ObjectMeta.Name, Namespace: instance.GetNamespace()}, currentService)
			if errors.IsNotFound(err) {
				desiredService.SetOwnerReferences([]metav1.OwnerReference{ownerRefSS(instance)})
				err = r.client.Create(context.Background(), &desiredService)
				currentService = &desiredService
			}
			if err != nil {
				return err
			}

			err = r.client.Get(context.Background(), types.NamespacedName{
				Name:      desiredService.ObjectMeta.Name,
				Namespace: instance.GetNamespace(),
			}, currentEndpoints)
			if err == nil {
				currentEndpoints.Subsets = desiredEndpoints.Subsets
				err = r.client.Update(context.Background(), currentEndpoints)
			}
			if errors.IsNotFound(err) {
				desiredEndpoints.SetOwnerReferences([]metav1.OwnerReference{ownerRefS(currentService), ownerRefSS(instance)})
				err = r.client.Create(context.Background(), &desiredEndpoints)
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getClusterName(nodes []corev1.Node) string {
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

	if _, hasLabel := node.Labels[gkeNodePoolLabel]; hasLabel {
		postfix := "-" + node.Labels[gkeNodePoolLabel]
		prefix := "gke-"
		clusterName := strings.Split(strings.TrimPrefix(node.Name, prefix), postfix)[0]
		log.Info(fmt.Sprintf("getClusterName: used a hack to determine the clusterName from hostname %s", node.Name))
		return clusterName
	}
	log.Info(fmt.Sprintf("getClusterName from %#v", node))
	panic("ClusterName could not be determined")
}

func keys(m interface{}) []string {
	mp, isMap := m.(map[string]interface{})
	if !isMap {
		return []string{}
	}
	keys := make([]string, 0, len(mp))
	for key := range mp {
		keys = append(keys, key)
	}
	return keys
}

func orElse(a, b interface{}) interface{} {
	if a == nil {
		return b
	}
	return a
}

func ownerRefSS(sync *mcv1.ServiceSync) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: sync.APIVersion,
		Kind:       sync.Kind,
		Name:       sync.GetName(),
		UID:        sync.GetUID(),
	}
}

func ownerRefS(sync *corev1.Service) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: "core/v1",
		Kind:       "Service",
		Name:       sync.GetName(),
		UID:        sync.GetUID(),
	}
}

func endpointsForHostsAndPort(nodes []corev1.Node) []mcv1.PeerEndpoint {
	var list = make([]mcv1.PeerEndpoint, len(nodes))
	for i, node := range nodes {
		for _, addr := range node.Status.Addresses {
			switch addr.Type {
			case corev1.NodeHostName:
				list[i].Hostname = addr.Address
			case corev1.NodeInternalIP:
				list[i].IPAddress = addr.Address
			}
		}
	}
	return list
}

// Builds up a Service & Endpoints as it should be created for the PeerService
func serviceForPeer(peerService mcv1.PeerService, namespace string) (corev1.Service, corev1.Endpoints) {
	serviceName := fmt.Sprintf("%s-%s", peerService.Cluster, peerService.ServiceName)
	service := corev1.Service{}
	service.ObjectMeta.Name = serviceName
	service.Namespace = namespace
	endpoints := corev1.Endpoints{}
	endpoints.ObjectMeta.Name = serviceName
	endpoints.Namespace = namespace

	service.Spec.Ports = make([]corev1.ServicePort, 0)
	addresses := make([]corev1.EndpointAddress, 0)
	ports := make([]corev1.EndpointPort, 0)

	for _, port := range peerService.Ports {
		name := fmt.Sprintf("%d", port.ExternalPort) // needs to be numeric (otherwise endpoints do not appear at 'kubectl describe service <name>')
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Port:       port.InternalPort,
			TargetPort: intstr.FromInt(int(port.ExternalPort)),
			Protocol:   "TCP",
			Name:       name,
		})
		ports = append(ports, corev1.EndpointPort{Port: port.ExternalPort, Name: name})
	}

	for _, host := range peerService.Endpoints {
		addresses = append(addresses, corev1.EndpointAddress{IP: host.IPAddress})
	}

	endpoints.Subsets = []corev1.EndpointSubset{
		corev1.EndpointSubset{Addresses: addresses, Ports: ports},
	}

	return service, endpoints
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func operatorStatusesEqual(a, b mcv1.ServiceSyncStatus) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b mcv1.PeerService) bool { return strings.Compare(a.ServiceName, b.ServiceName) < 0 }),
		cmpopts.SortSlices(func(a, b mcv1.PeerEndpoint) bool { return strings.Compare(a.IPAddress, b.IPAddress) < 0 }),
		cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 }),
		cmpopts.IgnoreFields(mcv1.ServiceSyncStatus{}, "LastPublishTime"),
	}
	if !cmp.Equal(a, b, conditionCmpOpts...) {
		// For debugging [operatorStatusesEqual], uncomment the following:
		// if diff := cmp.Diff(a, b, conditionCmpOpts...); diff != "" {
		// 	log.Info(fmt.Sprintf("Diff mismatch (-want +got):\n%s", diff))
		// }
		return false
	}
	return true
}
