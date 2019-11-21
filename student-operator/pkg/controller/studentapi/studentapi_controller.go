package studentapi

import (
	"bytes"
	"context"
	"fmt"
	"os"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	netgroupv1 "github.com/example-inc/memcached-operator/pkg/apis/netgroup/v1"
	"golang.org/x/crypto/ssh"
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

var log = logf.Log.WithName("controller_studentapi")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates two new StudentAPI Controllers and adds them to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconcilerCreate(mgr), newReconcilerDelete(mgr))
}

// newReconcilerCreate returns a new reconcile.Reconciler
func newReconcilerCreate(mgr manager.Manager) reconcile.Reconciler {
	return &CreateReconcileStudentAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerDelete returns a new reconcile.Reconciler
func newReconcilerDelete(mgr manager.Manager) reconcile.Reconciler {
	return &DeleteReconcileStudentAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds new Controllers to mgr
func add(mgr manager.Manager, rCreate reconcile.Reconciler, rDelete reconcile.Reconciler) error {
	// Create a new controller for Create event
	c_create, err := controller.New("create-controller", mgr, controller.Options{Reconciler: rCreate})
	if err != nil {
		return err
	}

	src_create := &source.Kind{Type: &netgroupv1.StudentAPI{}}

	h_create := &handler.EnqueueRequestForObject{}

	pred_create := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}

	// Watch for changes to primary resource StudentAPI
	err = c_create.Watch(src_create, h_create, pred_create)
	if err != nil {
		return err
	}

	// Create a new controller for Delete event
	c_delete, err := controller.New("delete-controller", mgr, controller.Options{Reconciler: rDelete})
	if err != nil {
		return err
	}

	src_delete := &source.Kind{Type: &netgroupv1.StudentAPI{}}

	h_delete := &handler.EnqueueRequestForObject{}

	pred_delete := predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
	}

	// Watch for changes to primary resource StudentAPI
	err = c_delete.Watch(src_delete, h_delete, pred_delete)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that CreateReconcileStudentAPI implements reconcile.Reconciler
var _ reconcile.Reconciler = &CreateReconcileStudentAPI{}

// CreateReconcileStudentAPI reconciles a StudentAPI object
type CreateReconcileStudentAPI struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

var _ reconcile.Reconciler = &DeleteReconcileStudentAPI{}

type DeleteReconcileStudentAPI struct {
	client client.Client
	scheme *runtime.Scheme
}

// this function connect as root to remote machine and create a new user named with his studentID
func AddUser(studentID string) (err error) {
	// Use SSH key authentication from the auth package
	// we ignore the host key
	clientConfig, _ := auth.PasswordKey("root", "root", ssh.InsecureIgnoreHostKey())

	// Create a new SCP client
	// TODO set dinamically address
	client := scp.NewClient("192.168.122.16:22", &clientConfig)

	// Connect to the remote server
	err = client.Connect()
	if err != nil {
		return
	}

	// Close client connection after the file has been copied
	defer client.Close()

	var b bytes.Buffer
	client.Session.Stdout = &b
	cmd := "./adduser.sh " + studentID
	if err = client.Session.Run(cmd); err != nil {
		return
	}
	fmt.Println(b.String())

	return nil

}

// this func connect through SSH to the user just created and copies the provided Public SSH Key
func CopySSHKey(studentID string) (err error) {
	// Use SSH key authentication from the auth package
	// we ignore the host key
	clientConfig, _ := auth.PasswordKey(studentID, "root", ssh.InsecureIgnoreHostKey())

	// Create a new SCP client
	// TODO set dinamically address
	client := scp.NewClient("192.168.122.16:22", &clientConfig)

	// Connect to the remote server
	err = client.Connect()
	if err != nil {
		return
	}

	// Open a file
	// TODO change file path and set home dinamically
	f, _ := os.Open("/home/davide/.ssh/id_rsa.pub")

	// Close client connection after the file has been copied
	defer client.Close()

	// Close the file after it has been copied
	defer f.Close()

	// Finaly, copy the file over
	// Usage: CopyFile(fileReader, remotePath, permission)
	// TODO change file path
	err = client.CopyFile(f, "./.ssh/authorized_keys", "0700")
	if err != nil {
		return
	}

	return nil
}

// Reconcile reads that state of the cluster for a StudentAPI object and makes changes based on the state read
// and what is in the StudentAPI.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *CreateReconcileStudentAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the StudentAPI instance
	instance := &netgroupv1.StudentAPI{}
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
	studentID := instance.Spec.ID
	/*studentName := instance.Spec.Name
	studentSurname := instance.Spec.Surname*/

	err = AddUser(studentID)
	if err != nil {
		errLogger := log.WithValues("Error", err)
		errLogger.Error(err, "Error")
	} else {
		err = CopySSHKey(studentID)
		if err != nil {
			errLogger := log.WithValues("Error", err)
			errLogger.Error(err, "Error")
		}
	}

	reqLogger := log.WithValues("Student ID", studentID)
	reqLogger.Info("Created new student")

	return reconcile.Result{}, nil
}

func (r *DeleteReconcileStudentAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Info("Deleted Student")
	// Fetch the StudentAPI instance
	instance := &netgroupv1.StudentAPI{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// This is the typical situation for a correctly deleted object
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	// if the object is correctly deleted this piece of code is never reached

	return reconcile.Result{}, nil
}
