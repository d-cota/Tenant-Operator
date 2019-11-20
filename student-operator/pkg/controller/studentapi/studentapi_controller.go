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
	corev1 "k8s.io/api/core/v1"
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

// Add creates a new StudentAPI Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileStudentAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("studentapi-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	src := &source.Kind{Type: &netgroupv1.StudentAPI{}}

	h := &handler.EnqueueRequestForObject{}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
	}

	// Watch for changes to primary resource StudentAPI
	err = c.Watch(src, h, pred)
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner StudentAPI
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &netgroupv1.StudentAPI{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileStudentAPI implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileStudentAPI{}

// ReconcileStudentAPI reconciles a StudentAPI object
type ReconcileStudentAPI struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
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

	// Open a file
	// TODO change file path and set home dinamically
	// f, _ := os.Open("/home/davide/try")

	// Close client connection after the file has been copied
	defer client.Close()

	// Close the file after it has been copied
	//defer f.Close()

	// Finaly, copy the file over
	// Usage: CopyFile(fileReader, remotePath, permission)

	// TODO change file path
	//err = client.CopyFile(f, "./.ssh/authorized_keys", "0644")

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
func (r *ReconcileStudentAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	//reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

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
	reqLogger := log.WithValues("Student ID", studentID)
	reqLogger.Info("Created new student")

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

	return reconcile.Result{}, nil
}
