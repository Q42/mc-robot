package servicesync

import (
	"context"
	"fmt"

	mcv1 "q42/mc-robot/pkg/apis/mc/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_servicesync")

// Add creates a new ServiceSync Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileServiceSync{client: mgr.GetClient(), scheme: mgr.GetScheme()}
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
	err = c.Watch(&source.Kind{Type: &mcv1.ServiceSync{}}, &handler.EnqueueRequestForObject{})
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
			// check if we are the owner
			if ownerRef := metav1.GetControllerOf(a.Meta); ownerRef != nil {
				log.Info(fmt.Sprintf("Service OwnerRef %#v", ownerRef))
			}

			log.Info(fmt.Sprintf("Reconciling for Service %#v", a))
			return []reconcile.Request{}
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
				nodeList, err = reconciler.getNodeList()
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

func (r *ReconcileServiceSync) getNodeList() (map[string]bool, error) {
	nodes := &corev1.NodeList{}
	err := r.client.List(context.TODO(), nodes)
	if err != nil {
		log.Error(err, "Error while responding to node event")
		return nil, err
	}

	schedulableMap := make(map[string]bool, len(nodes.Items))
	for _, node := range nodes.Items {
		schedulableMap[node.ObjectMeta.Name] = !node.Spec.Unschedulable
	}
	return schedulableMap, nil
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

// blank assignment to verify that ReconcileServiceSync implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileServiceSync{}

// ReconcileServiceSync reconciles a ServiceSync object
type ReconcileServiceSync struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
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

	// Shortcut for now
	if 1 > 0 {
		reqLogger.Info(fmt.Sprintf("Request is %#v", request))
		return reconcile.Result{}, nil
	}

	// Fetch the ServiceSync instance
	instance := &mcv1.ServiceSync{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

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
