package servicesync

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"

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
	eventTypeNormal  = "Normal" // kubernetes supports values Normal & Warning
)

var log = logf.Log.WithName("controller_servicesync")
var remoteStatus = make(map[string][]mcv1.PeerService, 0)
var pubSubTriggers = make(chan reconcile.Request, 0)

// Add creates a new ServiceSync Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileServiceSync{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
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
	nodes := &corev1.NodeList{}
	err = r.client.List(context.Background(), nodes)
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return nil, err
	}
	if len(nodes.Items) == 0 {
		log.Error(stderrors.New("No nodes found"), "Error while reading NodeList")
	}

	clusterName := getClusterName(nodes.Items[0])
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

func getClusterName(node corev1.Node) string {
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

// blank assignment to verify that ReconcileServiceSync implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileServiceSync{}

// ReconcileServiceSync reconciles a ServiceSync object
type ReconcileServiceSync struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a ServiceSync object and makes changes based on the state read
// and what is in the ServiceSync.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
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

	err = r.ensurePeerServices(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	sm, _ := r.getLocalServiceMap(instance)
	if instance.Status.LastPublishTime.IsZero() || instance.Status.LastPublishTime.Add(5*time.Minute).Before(time.Now()) {
		// published too long ago (or never)
		// so publish!
		// then reschedule reconcile after 5 minutes again
	}

	go func() {
		remoteStatus = map[string][]mcv1.PeerService{
			"other-cluster-2": cloneWithClusterName(sm, "other-cluster-2"),
			"other-cluster-3": cloneWithClusterName(sm, "hue-earth-3"),
		}
		time.Sleep(5 * time.Second)

		err := r.client.Get(ctx, request.NamespacedName, instance)
		panicOnError(err)
		oldStatus := instance.Status.DeepCopy()
		updateStatusFromRemoteState(instance, remoteStatus)
		if !operatorStatusesEqual(*oldStatus, instance.Status) {
			// update the Status of the resource with the special client.Status()-client (nothing happens when you don't use the sub-client):
			err = r.client.Status().Update(ctx, instance)
			panicOnError(err)
			r.recorder.Eventf(instance, eventTypeNormal, "PubSubMessage", "Remote status update received: need to reconcile local endpoints.")
		} else {
			reqLogger.Info("Status idential, nothing to do")
		}
	}()

	return reconcile.Result{}, nil

	// Worklist:
	// - Define Services for each of the PeerServices in the status
	// - Publish to PubSub
	// Elsewhere:
	// - Subscribe to PubSub & write to status

	// // Define a new Pod object
	// pod := newPodForCR(instance)

	// // Set ServiceSync instance as the owner and controller
	// import "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	// if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
	// 	return reconcile.Result{}, err
	// }

	// // Check if this Pod already exists
	// found := &corev1.Pod{}
	// err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	// if err != nil && errors.IsNotFound(err) {
	// 	reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
	// 	err = r.client.Create(context.TODO(), pod)
	// 	if err != nil {
	// 		return reconcile.Result{}, err
	// 	}

	// 	// Pod created successfully - don't requeue
	// 	return reconcile.Result{}, nil
	// } else if err != nil {
	// 	return reconcile.Result{}, err
	// }

	// // Pod already exists - don't requeue
	// reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	return reconcile.Result{}, nil
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

// // newPodForCR returns a busybox pod with the same name/namespace as the cr
// func newPodForCR(cr *mcv1.ServiceSync) *corev1.Pod {
// 	labels := map[string]string{
// 		"app": cr.Name,
// 	}
// 	return &corev1.Pod{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      cr.Name + "-pod",
// 			Namespace: cr.Namespace,
// 			Labels:    labels,
// 		},
// 		Spec: corev1.PodSpec{
// 			Containers: []corev1.Container{
// 				{
// 					Name:    "busybox",
// 					Image:   "busybox",
// 					Command: []string{"sleep", "3600"},
// 				},
// 			},
// 		},
// 	}
// }

func endpointsForHostsAndPort(nodes *corev1.NodeList) []mcv1.PeerEndpoint {
	var list = make([]mcv1.PeerEndpoint, len(nodes.Items))
	for i, node := range nodes.Items {
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

func cloneWithClusterName(ps []mcv1.PeerService, name string) []mcv1.PeerService {
	var cpy = make([]mcv1.PeerService, len(ps))
	for i := range ps {
		cpy[i] = mcv1.PeerService{
			Cluster:     name,
			ServiceName: ps[i].ServiceName,
			Endpoints:   ps[i].Endpoints,
			Ports:       ps[i].Ports,
		}
	}
	return cpy
}

func updateStatusFromRemoteState(s *mcv1.ServiceSync, ps map[string][]mcv1.PeerService) {
	clusters := make([]string, 0)
	for cluster := range ps {
		clusters = append(clusters, cluster)
	}
	s.Status.SelectedServices = make([]string, 0)
	s.Status.PeerClusters = clusters
	s.Status.PeerServices = ps
	s.Status.LastPublishTime = metav1.NewTime(time.Now())
}

func patchFromRemoteState(ps map[string][]mcv1.PeerService) client.Patch {
	clusters := make([]string, 0)
	for cluster := range ps {
		clusters = append(clusters, cluster)
	}
	mergePatch, err := json.Marshal(map[string]interface{}{
		"status": mcv1.ServiceSyncStatus{
			PeerClusters:     clusters,
			PeerServices:     ps,
			SelectedServices: []string{},
			LastPublishTime:  metav1.NewTime(time.Now()),
			LastPublishHash:  "",
		},
	})
	if err != nil {
		panic(err)
	}

	return client.ConstantPatch(types.MergePatchType, mergePatch)
}

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
