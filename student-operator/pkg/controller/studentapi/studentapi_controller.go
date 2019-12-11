package studentapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	netgroupv1 "github.com/example-inc/memcached-operator/pkg/apis/netgroup/v1"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
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

var finalizer string = "StudentFinalizer"

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// this function connect as root to remote machine and create a new user named with his studentID
func AddUser(studentID string, publicKey string) (err error) {

	key, err := ioutil.ReadFile("/home/davide/.ssh/id_rsa")
	if err != nil {
		return
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return
	}

	config := &ssh.ClientConfig{
		User: "bastion",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	bClient, err := ssh.Dial("tcp", "130.192.225.74:22", config)
	if err != nil {
		return
	}

	conn, err := bClient.Dial("tcp", "10.244.2.199:22")
	if err != nil {
		return
	}

	config = &ssh.ClientConfig{
		User: "davide",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, "10.244.2.199:22", config)
	if err != nil {
		return
	}

	sClient := ssh.NewClient(ncc, chans, reqs)

	session, err := sClient.NewSession()
	if err != nil {
		return
	}
	defer session.Close()

	file, err := os.Create("pkey")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, strings.NewReader(publicKey))
	if err != nil {
		return err
	}

	stat, _ := file.Stat()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		hostIn, _ := session.StdinPipe()
		defer hostIn.Close()
		fmt.Fprintf(hostIn, "C0664 %d %s\n", stat.Size(), "filecopyname")
		io.Copy(hostIn, file)
		fmt.Fprint(hostIn, "\x00")
		wg.Done()
	}()

	session.Run("/usr/bin/scp -t /remotedirectory/")
	wg.Wait()

	/*var b bytes.Buffer
	session.Stdout = &b
	cmd := "sudo ./addStudent.sh " + studentID

	if err = session.Run(cmd); err != nil {
		return
	}
	fmt.Println(b.String())*/

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
	// TODO read key from the yaml (?)
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

// TODO merge with AddUser
func DeleteUser(studentID string) (err error) {

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
	cmd := "pkill -KILL -u " + studentID + "; deluser --remove-home " + studentID
	if err = client.Session.Run(cmd); err != nil {
		return
	}
	fmt.Println(b.String())

	return nil

}

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
			//return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
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
			//return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !e.MetaNew.GetDeletionTimestamp().IsZero()
			//return false
			//return true
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

	// The object is being created, so if it does not have our finalizer,
	// then lets add the finalizer and update the object.
	// This is equivalent to register the finalizer
	if !containsString(instance.Finalizers, finalizer) {
		instance.Finalizers = append(instance.Finalizers, finalizer)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	err = AddUser(instance.Spec.ID, instance.Spec.PublicKey)
	if err != nil {
		errLogger := log.WithValues("Error", err)
		errLogger.Error(err, "Error")
	} else {
		err = CopySSHKey(instance.Spec.ID)
		if err != nil {
			errLogger := log.WithValues("Error", err)
			errLogger.Error(err, "Error")
		} else {
			reqLogger := log.WithValues("Student ID", instance.Spec.ID)
			reqLogger.Info("Created new student")
		}
	}

	return reconcile.Result{}, nil
}

func (r *DeleteReconcileStudentAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the StudentAPI instance
	instance := &netgroupv1.StudentAPI{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// We'll ignore not found errors since the object could
		// be already deleted
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// look for matching finalizers
	if containsString(instance.Finalizers, finalizer) {
		// if requested finalizer is present, we will handle the
		// deletion of external resource, i.e. a user account
		if err = DeleteUser(instance.Spec.ID); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried
			return reconcile.Result{}, err
		}

		// remove our finalizer from the list and update it.
		instance.Finalizers = removeString(instance.Finalizers, finalizer)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}

		reqLogger := log.WithValues("Student ID", instance.Spec.ID)
		reqLogger.Info("Deleted student")

	}

	return reconcile.Result{}, nil
}

// TODO deploy it on cluster
// TODO login on multiple machines (maybe watching the kubernetes label)
// TODO add label on CR to distinguish accessible machines
// TODO operator to manage the machines (and synchronize the access when a machine is created/deleted)
// TODO initializer e get stati utenti
// TODO handle all possible errors
// TODO refactor with class
// TODO testing
// TODO change auth method on server (no pass but key)
// TODO add name and surname when registering
