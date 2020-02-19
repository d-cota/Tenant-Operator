package host

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-yaml"
	netgroupv1 "github.com/netgroup/tenant-operator/pkg/apis/netgroup/v1"
	utils "github.com/netgroup/tenant-operator/utilities"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
	cm_log = logf.Log.WithName("controller_create_host")
	dm_log = logf.Log.WithName("controller_delete_host")
)

/* --- CONTROLLER --- */

// Add creates four new Tenant Controllers and adds them to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconcilerCreateHost(mgr), newReconcilerDeleteHost(mgr))
}

// newReconcilerHost returns a new reconcile.Reconciler
func newReconcilerCreateHost(mgr manager.Manager) reconcile.Reconciler {
	return &HostCreateReconcile{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerHost returns a new reconcile.Reconciler
func newReconcilerDeleteHost(mgr manager.Manager) reconcile.Reconciler {
	return &HostDeleteReconcile{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds new Controllers to mgr
// 2 controllers: Create a Configmap Host, Delete a Configmap Host
func add(mgr manager.Manager, rCreateHost reconcile.Reconciler, rDeleteHost reconcile.Reconciler) error {

	c_serv, err := controller.New("host-create-controller", mgr, controller.Options{Reconciler: rCreateHost})
	if err != nil {
		return err
	}

	// watch the ConfigMap
	src_serv := &source.Kind{Type: &v1.ConfigMap{}}

	kvp := utils.KeyValuePair{
		Key:   "use",
		Value: "Tenant",
	}

	h := &handler.EnqueueRequestForObject{}

	// events to watch, but reacts only to the ConfigMap labeled "use"="Tenant"
	pred_serv_c := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			labels := e.Meta.GetLabels()
			if val, ok := labels[kvp.Key]; ok {
				if val == kvp.Value {
					return true
				}
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			labels := e.MetaOld.GetLabels()
			if val, ok := labels[kvp.Key]; ok {
				// react only when the finalizer is not updated
				if val == kvp.Value && (len(e.MetaNew.GetFinalizers()) == len(e.MetaOld.GetFinalizers()) && e.MetaNew.GetDeletionTimestamp().IsZero()) {
					return true
				}
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	// Watch for changes to ConfigMaps
	err = c_serv.Watch(src_serv, h, pred_serv_c)
	if err != nil {
		return err
	}

	c_servDelete, err := controller.New("host-delete-controller", mgr, controller.Options{Reconciler: rDeleteHost})
	if err != nil {
		return err
	}

	pred_serv_d := predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			labels := e.Meta.GetLabels()
			if val, ok := labels[kvp.Key]; ok {
				if val == kvp.Value {
					return true
				}
			}
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			labels := e.MetaOld.GetLabels()
			if val, ok := labels[kvp.Key]; ok {
				if val == kvp.Value && !e.MetaNew.GetDeletionTimestamp().IsZero() {
					return true
				}
			}
			return false
		},
	}

	// Watch for changes to ConfigMaps
	err = c_servDelete.Watch(src_serv, h, pred_serv_d)
	if err != nil {
		return err
	}

	return nil
}

type HostCreateReconcile struct {
	client client.Client
	scheme *runtime.Scheme
}

type HostDeleteReconcile struct {
	client client.Client
	scheme *runtime.Scheme
}

/* --- RECONCILERS --- */

// Reconcile reconciles a Host ConfigMap when it is created. It looks for all the tenant
// with the matching Role field and it adds them to the machine
func (r *HostCreateReconcile) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Tenant instance
	instance := &v1.ConfigMap{}
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

	// add ConfigMap finalizer
	if !utils.ContainsString(instance.Finalizers, utils.C_FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, utils.C_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	yml := instance.Data["config"]

	var config utils.Config

	// read the yaml in the ConfigMap
	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		cm_log.Error(err, err.Error())
	}

	// list all the tenant CRs in the namespace
	users := &netgroupv1.TenantList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	err = r.client.List(context.TODO(), users, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all objects
	for _, user := range users.Items {
		// iterate over roles of one object
		for _, role := range user.Info.Roles {
			// if user role match with at least one of the host
			// authorized role then utils.AddUser()

			// iterate over ConfigMap roles
			if utils.ContainsString(config.Roles, role) {
				// sets desired state of the object
				if !utils.ContainsString(user.Spec.Hosts, config.Remoteaddr) {
					user.Spec.Hosts = append(user.Spec.Hosts, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), &user); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := utils.Connection{
					RemoteAddr: config.Remoteaddr,
					RemoteUser: config.Remoteuser,
					RemotePort: config.Remoteport,
					PublicKey:  user.Info.PublicKey,
					NewUser:    user.Info.ID,
				}

				// if the user is not present in the machine (Spec is not empty, Stat it is), then add him
				if !utils.ContainsString(user.Stat.Hosts, config.Remoteaddr) {
					err = utils.AddUser(conn, cm_log)
					if err != nil {
						// if any error occurs, reconcile after 30 sec
						cm_log.Error(err, err.Error())

						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						user.Stat.Hosts = append(user.Stat.Hosts, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), &user); err != nil {
							return reconcile.Result{}, err
						}
						cm_log.Info(fmt.Sprintf("User %s added to %s", user.Info.ID, config.Remoteaddr))
						break
					}
				} else {
					cm_log.Info(fmt.Sprintf("User %s already present in %s", user.Info.ID, config.Remoteaddr))
					break
				}
			}
		}
	}

	cm_log.Info(fmt.Sprintf("Host at %s correctly created", config.Remoteaddr))

	return reconcile.Result{}, nil
}

// Reconcile reconcile a ConfigMap Host at the deletion. It deletes all accounts in the machine
func (r *HostDeleteReconcile) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Tenant instance
	instance := &v1.ConfigMap{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// We'll ignore not found errors since the object could
		// be already deleted

		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	yml := instance.Data["config"]

	var config utils.Config

	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		dm_log.Error(err, err.Error())
	}

	users := &netgroupv1.TenantList{}

	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}

	err = r.client.List(context.TODO(), users, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all objects
	for _, user := range users.Items {
		// iterate over roles of one object
		for _, role := range user.Info.Roles {
			// iterate over ConfigMap roles
			if utils.ContainsString(config.Roles, role) {

				if utils.ContainsString(user.Spec.Hosts, config.Remoteaddr) {
					// delete Spec host address entry in the user CR
					user.Spec.Hosts = utils.RemoveString(user.Spec.Hosts, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), &user); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := utils.Connection{
					RemoteAddr: config.Remoteaddr,
					RemoteUser: config.Remoteuser,
					RemotePort: config.Remoteport,
					PublicKey:  user.Info.PublicKey,
					NewUser:    user.Info.ID,
				}

				if utils.ContainsString(user.Stat.Hosts, config.Remoteaddr) {
					err = utils.DeleteUser(conn, dm_log)
					if err != nil {
						dm_log.Error(err, err.Error())

						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// delete Stat host address entry in the user CR
						user.Stat.Hosts = utils.RemoveString(user.Stat.Hosts, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), &user); err != nil {
							return reconcile.Result{}, err
						}
						dm_log.Info(fmt.Sprintf("User %s deleted from %s", user.Info.ID, config.Remoteaddr))
						break
					}

				} else {
					dm_log.Info(fmt.Sprintf("User %s already deleted from %s", user.Info.ID, config.Remoteaddr))
					break
				}
			}
		}
	}

	// looks for matching finalizers
	if utils.ContainsString(instance.Finalizers, utils.C_FINALIZER) {
		// remove our finalizer from the list and update it.
		instance.Finalizers = utils.RemoveString(instance.Finalizers, utils.C_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	dm_log.Info(fmt.Sprintf("Host at %s correctly deleted", config.Remoteaddr))

	return reconcile.Result{}, nil
}