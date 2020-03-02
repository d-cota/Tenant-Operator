package tenant

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
	netgroupv1 "github.com/netgroup/tenant-operator/pkg/apis/netgroup/v1"
	utils "github.com/netgroup/tenant-operator/utilities"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

var (
	// a logger for each controller
	c_log    = logf.Log.WithName("controller_create_tenant")
	d_log    = logf.Log.WithName("controller_delete_tenant")
	machines []string
	config   utils.Config
)

/* --- CONTROLLER --- */
// Add creates four new Tenant Controllers and adds them to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconcilerCreate(mgr), newReconcilerDelete(mgr))
}

// newReconcilerCreate returns a new reconcile.Reconciler
func newReconcilerCreate(mgr manager.Manager) reconcile.Reconciler {
	return &CreateReconcileTenant{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerDelete returns a new reconcile.Reconciler
func newReconcilerDelete(mgr manager.Manager) reconcile.Reconciler {
	return &DeleteReconcileTenant{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds new Controllers to mgr
// 2 controllers: Create a Tenant, Delete a Tenant
func add(mgr manager.Manager, rCreate reconcile.Reconciler, rDelete reconcile.Reconciler) error {
	// Create a new controller for Create event
	c_create, err := controller.New("create-controller", mgr, controller.Options{Reconciler: rCreate})
	if err != nil {
		return err
	}

	// watch Tenant object
	src_Tenant := &source.Kind{Type: &netgroupv1.Tenant{}}

	h := &handler.EnqueueRequestForObject{}

	// which events has to watch the controller
	pred_create := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
	}

	// Watch for changes to primary resource Tenant
	err = c_create.Watch(src_Tenant, h, pred_create)
	if err != nil {
		return err
	}

	// Create a new controller for Delete event
	c_delete, err := controller.New("delete-controller", mgr, controller.Options{Reconciler: rDelete})
	if err != nil {
		return err
	}

	pred_delete := predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false

		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// since a finalizer is present, reacts when the DeletionTimestamp is not zero
			return !e.MetaNew.GetDeletionTimestamp().IsZero()
		},
	}

	// Watch for changes to primary resource Tenant
	err = c_delete.Watch(src_Tenant, h, pred_delete)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that CreateReconcileTenant implements reconcile.Reconciler
var _ reconcile.Reconciler = &CreateReconcileTenant{}

// CreateReconcileTenant reconciles a Tenant object
type CreateReconcileTenant struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apihost
	client client.Client
	scheme *runtime.Scheme
}

var _ reconcile.Reconciler = &DeleteReconcileTenant{}

type DeleteReconcileTenant struct {
	client client.Client
	scheme *runtime.Scheme
}

/* --- RECONCILERS --- */

// Reconcile reads that state of the cluster for a Tenant object and makes changes based on the state read
// and what is in the Tenant.Info
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
// Reconcile reconciles a Tenant object when it is created. It adds the tenant to
// all the authorized machines watching at Role field.
func (r *CreateReconcileTenant) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Tenant instance
	instance := &netgroupv1.Tenant{}
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

	// The object is being created, so if it does not have our finalizer,
	// then lets add the finalizer and update the object.
	// This is equivalent to register the finalizer
	if !utils.ContainsString(instance.Finalizers, utils.S_FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, utils.S_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	// list all Host ConfigMap
	cmaps := &v1.ConfigMapList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string]string{"use": "Tenant"}),
	}
	err = r.client.List(context.TODO(), cmaps, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all cmaps
	for _, cmap := range cmaps.Items {
		yml := cmap.Data["config"]

		if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
			c_log.Error(err, err.Error())
		}

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the host
			// authorized roles then utils.AddUser()

			// iterate over ConfigMap roles
			if utils.ContainsString(config.Roles, role) {
				// sets desired state of the object
				if !utils.ContainsString(instance.Spec.Hosts, config.Remoteaddr) {
					instance.Spec.Hosts = append(instance.Spec.Hosts, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := utils.Connection{
					RemoteAddr: config.Remoteaddr,
					RemoteUser: config.Remoteuser,
					RemotePort: config.Remoteport,
					PublicKey:  instance.Info.PublicKey,
					NewUser:    instance.Info.ID,
				}

				if !utils.ContainsString(instance.Stat.Hosts, config.Remoteaddr) {
					err = utils.AddUser(conn, c_log)
					if err != nil {
						c_log.Error(err, err.Error())

						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						instance.Stat.Hosts = append(instance.Stat.Hosts, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), instance); err != nil {
							return reconcile.Result{}, err
						}
						machines = append(machines, config.Remoteaddr)
						c_log.Info(fmt.Sprintf("User %s added to %s", instance.Info.ID, config.Remoteaddr))
						break
					}
				} else {
					c_log.Info(fmt.Sprintf("User %s already present in %s", instance.Info.ID, config.Remoteaddr))
					break
				}
			}
		}
	}

	// look for vpn pod
	pods := &v1.PodList{}
	opts = []client.ListOption{
		client.MatchingLabels(map[string]string{"app": "openvpn", "release": utils.POD_RELEASE}),
	}
	err = r.client.List(context.TODO(), pods, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// look for vpn service
	svc := &v1.ServiceList{}
	opts = []client.ListOption{
		client.MatchingLabels(map[string]string{"app": "openvpn", "release": utils.SERVICE_RELEASE}),
	}
	err = r.client.List(context.TODO(), svc, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	if len(pods.Items) > 0 && len(svc.Items) > 0 {
		// generate the user VPN certificate
		err = utils.GenerateVPNCert(instance.Info.ID, pods.Items[0].Name, svc.Items[0].Status.LoadBalancer.Ingress[0].IP, c_log)
		if err != nil {
			c_log.Error(err, err.Error())
		}
	}

	// send an email with instructions
	err = utils.SendEmail(instance.Info.Email, machines)
	if err != nil {
		c_log.Error(err, err.Error())
	}

	reqLogger := c_log.WithValues("User ID", instance.Info.ID)
	reqLogger.Info("Correctly created new user")

	return reconcile.Result{}, nil
}

// Reconcile reconcile a Tenant object when it is deleted. It handles the account deletion
// from hosts and revocates the ovpn certificate
func (r *DeleteReconcileTenant) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Tenant instance
	instance := &netgroupv1.Tenant{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// We'll ignore not found errors since the object could
		// be already deleted

		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// look for Host ConfigMap
	cmaps := &v1.ConfigMapList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string]string{"use": "Tenant"}),
	}
	err = r.client.List(context.TODO(), cmaps, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all cmaps
	for _, cmap := range cmaps.Items {
		yml := cmap.Data["config"]

		if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
			d_log.Error(err, err.Error())
		}

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the host
			// authorized roles then delete user from that machine

			// iterate over ConfigMap roles
			if utils.ContainsString(config.Roles, role) {
				// sets desired state of the object
				if utils.ContainsString(instance.Spec.Hosts, config.Remoteaddr) {
					instance.Spec.Hosts = utils.RemoveString(instance.Spec.Hosts, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := utils.Connection{
					RemoteAddr: config.Remoteaddr,
					RemoteUser: config.Remoteuser,
					RemotePort: config.Remoteport,
					PublicKey:  instance.Info.PublicKey,
					NewUser:    instance.Info.ID,
				}

				if utils.ContainsString(instance.Stat.Hosts, config.Remoteaddr) {
					err = utils.DeleteUser(conn, d_log)
					if err != nil {
						d_log.Error(err, err.Error())

						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						instance.Stat.Hosts = utils.RemoveString(instance.Stat.Hosts, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), instance); err != nil {
							return reconcile.Result{}, err
						}
						d_log.Info(fmt.Sprintf("User %s deleted from %s", instance.Info.ID, config.Remoteaddr))
						break
					}
				} else {
					d_log.Info(fmt.Sprintf("User %s already deleted from %s", instance.Info.ID, config.Remoteaddr))
					break
				}
			}
		}
	}

	namespace, _ := k8sutil.GetWatchNamespace()
	// name of the secret containing the certificate to be deleted
	name := types.NamespacedName{Namespace: namespace, Name: instance.Info.ID + "-ovpn"}
	secret := &v1.Secret{}
	err = r.client.Get(context.TODO(), name, secret)
	if err != nil {
		d_log.Error(err, err.Error())
	} else {
		// delete the ovpn certificate
		err = r.client.Delete(context.TODO(), secret)
		if err != nil {
			d_log.Error(err, err.Error())
		}
	}

	// look for matching finalizers
	if utils.ContainsString(instance.Finalizers, utils.S_FINALIZER) {
		// remove our finalizer from the list and update it.
		instance.Finalizers = utils.RemoveString(instance.Finalizers, utils.S_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	reqLogger := d_log.WithValues("Tenant ID", instance.Info.ID)
	reqLogger.Info("Correctly deleted user")

	return reconcile.Result{}, nil
}
