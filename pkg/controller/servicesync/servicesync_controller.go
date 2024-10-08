package servicesync

import (
	"context"
	"encoding/json"
	coreErrors "errors"
	"fmt"
	"sort"
	"time"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"q42/mc-robot/pkg/datasource"

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
	controllerName          = "mc_robot"
	eventTypeNormal         = "Normal"  // kubernetes supports values Normal & Warning
	eventTypeWarning        = "Warning" // kubernetes supports values Normal & Warning
	broadcastRequestPayload = "broadcastRequest"
	ownerReferenceUIDField  = "metadata.ownerReferences[].uid"
)

var log = logf.Log.WithName("controller_servicesync")
var clusterName string
var clusterLastUpdateTimes = make(map[string]time.Time, 0)

// Add creates a new ServiceSync Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, es datasource.ExternalSource) error {
	return add(mgr, newReconciler(mgr, es))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, es datasource.ExternalSource) reconcile.Reconciler {
	// Adds ownerRefernce fieldIndexer
	mgr.GetFieldIndexer().IndexField(&corev1.Service{}, ownerReferenceUIDField, func(o runtime.Object) []string {
		var res = make([]string, 0)
		for _, owner := range (o.(*corev1.Service)).OwnerReferences {
			res = append(res, string(owner.UID))
		}
		return res
	})

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
					log.Info("Found elegible Service for ServiceSync", "service", service.Name, "servicesync", sync.Name)
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
						Name:      sync.GetName(),
						Namespace: sync.GetNamespace(),
					}})
				}
			}

			// For any Service that a ServiceSync is the owner of, enqueue the owner ServiceSync
			if ownerRef := metav1.GetControllerOf(a.Meta); ownerRef != nil {
				log.Info("Found ServiceSync owned Service", "service", service.Name, "servicesync", ownerRef.Name)
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

	// Update a metric every 5s indicating how long ago the cluster configuration was received
	go func() {
		for {
			var min time.Duration = -1
			var max time.Duration = -1
			if len(clusterLastUpdateTimes) > 0 {
				for _, when := range clusterLastUpdateTimes {
					delta := time.Now().Sub(when)
					if min < 0 {
						min = delta
						max = delta
					}
					if delta < min {
						min = delta
					}
					if max < delta {
						max = delta
					}
				}
				metricUpdateMinAge.Set(min.Seconds())
				metricUpdateMaxAge.Set(max.Seconds())
			}
			time.Sleep(5 * time.Second)
		}
	}()

	return nil
}

var lastRequestedBroadcast = time.Time{}

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
	reqLogger.Info("Reconciling ServiceSync", "namespace", request.Namespace, "servicesync", request.Name)
	ctx := context.Background()

	clusterName := r.getClusterName()

	// Fetch the ServiceSync instance
	instance := &mcv1.ServiceSync{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request. Return and don't requeue.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// Broadcast once after coming online
	if time.Since(lastRequestedBroadcast) > time.Hour {
		lastRequestedBroadcast = time.Now()
		r.es.Publish(instance.Spec.TopicURL, []byte(broadcastRequestPayload), clusterName)
	}

	// Subscribe to PeerService changes from other clusters
	r.es.Subscribe(instance.Spec.TopicURL, r.callbackFor(request.NamespacedName))
	err = r.ensurePeerServices(instance)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}

	// Compute our local services & save optionally
	selfStatus, hasChanged, err := r.ensureLocalStatus(instance)
	if err != nil {
		reqLogger.Error(err, err.Error())
		return reconcile.Result{}, err
	}
	instance.Status.Clusters[r.getClusterName()] = &selfStatus

	// Publish our PeerService's to other clusters
	if hasChanged || selfStatus.LastUpdate.IsZero() || selfStatus.LastUpdate.Add(interval(instance)).Before(time.Now()) {
		// published too long ago (or never), so publish!
		reqLogger.Info("Publishing local services")
		res, err := r.publish(instance)
		return res, err
	}

	return reconcile.Result{RequeueAfter: time.Until(selfStatus.LastUpdate.Time.Add(interval(instance)))}, nil
}

func interval(instance *mcv1.ServiceSync) time.Duration {
	reconcileIntervalString := instance.Spec.ReconcileInterval
	if reconcileIntervalString == "" {
		reconcileIntervalString = "300s"
	}
	interval, err := time.ParseDuration(instance.Spec.ReconcileInterval)
	if err != nil {
		return time.Second * 300
	}
	return interval
}

func (r *ReconcileServiceSync) publish(instance *mcv1.ServiceSync) (reconcile.Result, error) {
	clusterName := r.getClusterName()
	cluster := instance.Status.Clusters[clusterName]
	metricServicesExposed.Set(float64(len(cluster.Services)))

	if clusterName == "" {
		return reconcile.Result{RequeueAfter: interval(instance)}, errors.NewInternalError(nil)
	}

	jsonData, err := json.Marshal(map[string]map[string]*mcv1.PeerService{clusterName: cluster.Services})
	if err != nil {
		return reconcile.Result{}, err
	}

	r.es.Publish(instance.Spec.TopicURL, jsonData, clusterName)
	err = r.updateAndSetPublishTime(instance)
	logOnError(err, "Failed to update time on ServiceSync")

	// then reschedule reconcile after 5 minutes again
	return reconcile.Result{RequeueAfter: interval(instance)}, nil
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

// This callback parses and then writes the data of remote PeerServices to our cluster, using ensureRemoteStatus
func (r *ReconcileServiceSync) callbackFor(name types.NamespacedName) func([]byte, string) {
	return func(dataJson []byte, from string) {
		logger := log.WithValues("sender", from)
		sync := &mcv1.ServiceSync{}

		if from == r.getClusterName() {
			// If this is our own message, only run the cleanup (if we receive our own message we know for sure that PubSub is working so we can prune other clusters)
			err := r.client.Get(context.Background(), name, sync)
			if err == nil {
				err = r.ensurePruned(name, sync)
				logOnError(err, "Failed to prune ServiceSync")
			}
			return
		}

		// Handle broadcast requests
		if string(dataJson) == broadcastRequestPayload {
			sync := &mcv1.ServiceSync{}
			err := r.client.Get(context.Background(), name, sync)
			if err == nil {
				_, err = r.publish(sync)
			}
			logOnError(err, "Failed to broadcast ServiceSync")
			return
		}

		// Unmarshal data
		freshData := map[string]map[string]*mcv1.PeerService{}
		err := json.Unmarshal(dataJson, &freshData)
		if err != nil {
			logger.Error(err, "Got JSON from peer that could not be unmarshalled to PeerServices")
			return
		}

		if len(freshData) > 1 || len(freshData) < 1 {
			logger.Error(coreErrors.New("JSON map should contain 1 clusters information"), "Weird state, expecting 1 cluster in JSON from peer")
			return
		}

		clusterName := keys(freshData)[0]
		cluster := mcv1.Cluster{
			Name:       clusterName,
			Services:   freshData[clusterName],
			LastUpdate: metav1.NewTime(time.Now()),
		}

		// Save
		err = r.ensureRemoteStatus(name, cluster)
		logOnError(err, "ensureRemoteStatus")

		// Also cleanup expired entries, after writing new data
		if err == nil {
			err = r.ensurePruned(name, sync)
			logOnError(err, "Failed to prune ServiceSync")
		}
	}
}

func (r *ReconcileServiceSync) ensureLocalStatus(instance *mcv1.ServiceSync) (current mcv1.Cluster, hasChanged bool, err error) {
	// Collect previous state
	if instance.Status.Clusters == nil {
		instance.Status.Clusters = make(map[string]*mcv1.Cluster, 0)
	}
	if instance.Status.Clusters[clusterName] == nil {
		instance.Status.Clusters[clusterName] = &mcv1.Cluster{Name: clusterName, Services: make(map[string]*mcv1.PeerService, 0)}
	}
	selfStatus := instance.Status.Clusters[clusterName]

	// Compute current state
	current.Name = r.getClusterName()
	current.LastUpdate = selfStatus.LastUpdate
	current.Services, err = r.getLocalServiceMap(instance)
	if err != nil {
		return
	}

	// Diff & save optionally
	isEqual, diff := operatorPeerServicesEqual(selfStatus.Services, current.Services)
	hasChanged = !isEqual
	serviceNames := keys(current.Services)
	sort.Strings(serviceNames)
	logger := log.WithValues("serviceNames", serviceNames, "diff", diff, "servicesync", instance.Name)

	if !isEqual {
		logger.Info("Local services changed, updating")
		original := instance.DeepCopy()
		instance.Status.Clusters[clusterName].Services = current.Services
		err = r.client.Status().Patch(context.Background(), instance, client.MergeFrom(original))
		if err != nil {
			logger.Error(err, "Local status patch failed")
			r.recorder.Eventf(instance, eventTypeNormal, "EnsuringLocalStatus", "Local status patch failed: %s", err)
			return
		}
		logger.Info("Local services patched")
		r.recorder.Eventf(instance, eventTypeNormal, "EnsuringLocalStatus", "Local status patched")
	} else {
		r.recorder.Eventf(instance, eventTypeNormal, "EnsuringLocalStatus", "Local status identical, no action")
		logger.Info("Local status identical, no action")
	}
	return
}

// Writing the remote status to the local ServiceSync.Status object
func (r *ReconcileServiceSync) ensureRemoteStatus(name types.NamespacedName, cluster mcv1.Cluster) error {
	ctx := context.Background()

	// Load latest state
	instance := &mcv1.ServiceSync{}
	err := r.client.Get(ctx, name, instance)
	if err != nil {
		return err
	}
	originalInstance := instance.DeepCopy()

	// Modify
	if instance.Status.Clusters == nil {
		instance.Status.Clusters = make(map[string]*mcv1.Cluster, 0)
	}
	instance.Status.Clusters[cluster.Name] = &cluster
	instance.Status.Peers = filterOut(keys(instance.Status.Clusters), r.getClusterName())

	return r.save(name, originalInstance, instance)
}

// Writing the remote status to the local ServiceSync.Status object
func (r *ReconcileServiceSync) ensurePruned(name types.NamespacedName, instance *mcv1.ServiceSync) error {
	logger := log.WithValues("servicesync", name.Name)

	// Save original state
	originalInstance := instance.DeepCopy()
	if instance.Status.Clusters == nil {
		instance.Status.Clusters = make(map[string]*mcv1.Cluster, 0)
	}

	// Prune old/expired clusters
	pruned := PruneExpired(&instance.Status.Clusters, instance.Spec.PrunePeerAtAge)
	if len(pruned) > 0 {
		logger.Info("Pruned remote clusters", "clusters", pruned)
		r.recorder.Eventf(instance, eventTypeNormal, "PrunedClusters", "Pruned remote clusters %s", pruned)
	}

	return r.save(name, originalInstance, instance)
}

// Writing the remote status to the local ServiceSync.Status object
func (r *ReconcileServiceSync) save(name types.NamespacedName, originalInstance *mcv1.ServiceSync, instance *mcv1.ServiceSync) error {
	ctx := context.Background()
	logger := log.WithValues("servicesync", name.Name)

	// Patch if necessary
	if !operatorStatusesEqual(originalInstance.Status, instance.Status) {
		// update the Status of the resource with the special client.Status()-client (nothing happens when you don't use the sub-client):
		err := r.client.Status().Patch(ctx, instance, client.MergeFrom(originalInstance))
		if err == nil {
			logger.Info("Patched ServiceSync status")
			r.recorder.Eventf(instance, eventTypeNormal, "EnsuringRemoteStatus", "Remote status patched")
		} else {
			logger.V(400).Info("Patching status of ServiceSync failed", "error", err)
			r.recorder.Eventf(instance, eventTypeWarning, "EnsuringRemoteStatus", "Remote status patch failed: %s", err)
		}
		return err
	}
	r.recorder.Eventf(instance, eventTypeNormal, "EnsuringRemoteStatus", "Remote status identical, no action")
	logger.Info("Status identical, nothing to do")
	return nil
}

func (r *ReconcileServiceSync) updateAndSetPublishTime(instance *mcv1.ServiceSync) error {
	original := instance.DeepCopy()
	instance.Status.Clusters[r.getClusterName()].LastUpdate = metav1.NewTime(time.Now())
	return r.client.Status().Patch(context.Background(), instance, client.MergeFrom(original))
}

// This takes the existing ServiceSync.Status and creates the Service's and Endpoints for the remote clusters
func (r *ReconcileServiceSync) ensurePeerServices(instance *mcv1.ServiceSync) error {
	existingServices := &corev1.ServiceList{}
	err := r.client.List(context.Background(), existingServices, client.MatchingFields{ownerReferenceUIDField: string(instance.UID)})
	if err != nil {
		log.Error(err, "Error while computing ServiceMap")
		return err
	}
	var preexistingServicesMap = make(map[string]corev1.Service, 0)
	for _, service := range existingServices.Items {
		preexistingServicesMap[service.ObjectMeta.Name] = service
	}

	// Collect all peerServices that we need to configure
	var peerServices []mcv1.PeerService
	for _, cluster := range instance.Status.Clusters {
		if cluster.Name == "" || cluster.Name == r.getClusterName() {
			continue
		}
		for _, service := range cluster.Services {
			if len(service.Ports) == 0 {
				continue
			}
			peerServices = append(peerServices, *service)
		}
	}

	// Configure all peerServices
	metricServicesConfigured.Set(float64(len(peerServices)))
	var multiError []error
	for _, peerService := range peerServices {
		desiredService, desiredEndpoints := serviceForPeer(peerService, instance.GetNamespace())
		delete(preexistingServicesMap, desiredService.ObjectMeta.Name)
		currentService := &corev1.Service{}
		currentEndpoints := &corev1.Endpoints{}
		annotations := map[string]string{
			"owner":               instance.Name,
			"remote-cluster":      peerService.Cluster,
			"remote-service-name": peerService.ServiceName,
		}

		err := r.client.Get(context.Background(), types.NamespacedName{Name: desiredService.ObjectMeta.Name, Namespace: instance.GetNamespace()}, currentService)
		// Update if it changed (or we add annotations in subsequent releases)
		if err == nil && !mapContains(currentService.Annotations, annotations) {
			currentService.SetAnnotations(mapMerge(currentService.Annotations, annotations))
			currentService.SetOwnerReferences([]metav1.OwnerReference{ownerRefSS(instance)})
			err = r.client.Update(context.Background(), currentService)
		}
		// Create if it does not exist
		if errors.IsNotFound(err) {
			desiredService.SetOwnerReferences([]metav1.OwnerReference{ownerRefSS(instance)})
			desiredService.SetAnnotations(annotations)
			err = r.client.Create(context.Background(), &desiredService)
			currentService = &desiredService
			err = nil
		}

		if err != nil {
			multiError = append(multiError, err)
			continue
		}

		// Upsert operation:
		// try to get the resource...
		err = r.client.Get(context.Background(), types.NamespacedName{
			Name:      desiredService.ObjectMeta.Name,
			Namespace: instance.GetNamespace(),
		}, currentEndpoints)
		// if that works, update,
		if err == nil {
			currentEndpoints.Subsets = desiredEndpoints.Subsets
			err = r.client.Update(context.Background(), currentEndpoints)
		}
		// ... or else create it
		if errors.IsNotFound(err) {
			desiredEndpoints.SetOwnerReferences([]metav1.OwnerReference{ownerRefS(currentService), ownerRefSS(instance)})
			err = r.client.Create(context.Background(), &desiredEndpoints)
		}
		if err != nil {
			multiError = append(multiError, err)
			continue
		}
	}

	// Any service that has not been ensured above & removed by the delete from preexistingServicesMap is extra and should be deleted
	if len(preexistingServicesMap) > 0 {
		log.Info("Found preexisting services, which will be delted now", "prunedservices", keys(preexistingServicesMap))
		for _, service := range preexistingServicesMap {
			log.Info("Deleting service", "service", service.ObjectMeta.Name)
			err := r.client.Delete(context.Background(), &service)
			if err != nil && !errors.IsNotFound(err) {
				multiError = append(multiError, err)
			}
		}
	}

	if len(multiError) == 1 {
		return multiError[0]
	}
	if len(multiError) > 0 {
		return fmt.Errorf("Multiple errors during ensurePeerServices: %v", multiError)
	}

	return nil
}

var boolTrue = true

func ownerRefSS(sync *mcv1.ServiceSync) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         sync.APIVersion,
		Kind:               sync.Kind,
		Name:               sync.GetName(),
		UID:                sync.GetUID(),
		BlockOwnerDeletion: &boolTrue,
		Controller:         &boolTrue,
	}
}

func ownerRefS(sync *corev1.Service) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         "core/v1",
		Kind:               "Service",
		Name:               sync.GetName(),
		UID:                sync.GetUID(),
		BlockOwnerDeletion: &boolTrue,
	}
}

// PruneExpired removes clusters from the list if they are LastUpdated too long ago
func PruneExpired(clusters *map[string]*mcv1.Cluster, maxAgeStr string) (pruned []string) {
	maxAge, err := time.ParseDuration(maxAgeStr)
	if maxAgeStr != "" && err == nil && maxAge > 0 {
		for clusterName, cluster := range *clusters {
			if cluster.LastUpdate.Time.Before(time.Now().Add(-1 * maxAge)) {
				delete(*clusters, clusterName)
				pruned = append(pruned, clusterName)
			}
		}
	}
	return
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
		if host.IPAddress == "" {
			log.Info("empty IP address, skipping", "host", host.Hostname)
			continue
		}
		addresses = append(addresses, corev1.EndpointAddress{IP: host.IPAddress})
	}

	endpoints.Subsets = []corev1.EndpointSubset{
		{Addresses: addresses, Ports: ports},
	}

	return service, endpoints
}

// Ensure that the array becomes up to date: patch-fn is called for every entry of keys.
// If keys contains entries that don't exist in the array, the array will be updated to
// include those clusters too.
func patchClusters(clusters map[string]mcv1.Cluster, keys []string, patch func(cluster mcv1.Cluster) mcv1.Cluster) map[string]mcv1.Cluster {
	toHandle := make(map[string]bool, 0)
	for _, key := range keys {
		toHandle[key] = true
	}
	for i, c := range clusters {
		if toHandle[c.Name] {
			clusters[i] = patch(c)
			toHandle[c.Name] = false
		}
	}
	for name, needsHandling := range toHandle {
		if needsHandling {
			clusters[name] = patch(mcv1.Cluster{Name: name})
		}
	}
	return clusters
}
