package studentapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

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

type Connection struct {
	remoteAddr string
	remotePort string
	remoteUser string
	publicKey  string
	newUser    string
}

const (
	PRIVATE_KEY  string = "/home/davide/.ssh/id_rsa"
	BASTION      string = "bastion"
	BASTION_ADDR string = "130.192.225.74:22"
	HOME         string = "/home/"
	FINALIZER    string = "StudentFinalizer"
)

func EstablishConnection(remoteAddr string, remotePort string, remoteUser string) (*ssh.Session, error) {
	key, err := ioutil.ReadFile(PRIVATE_KEY) // path to bastion private key authentication
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: BASTION,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	bClient, err := ssh.Dial("tcp", BASTION_ADDR, config)
	if err != nil {
		return nil, err
	}

	rAddr := remoteAddr + ":" + remotePort
	conn, err := bClient.Dial("tcp", rAddr) // start dialing with remote server
	if err != nil {
		return nil, err
	}

	config = &ssh.ClientConfig{
		User: remoteUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, rAddr, config)
	if err != nil {
		return nil, err
	}

	sClient := ssh.NewClient(ncc, chans, reqs)

	return sClient.NewSession()
}

// this function connect as root to remote machine and create a new user named with his studentID
func AddUser(c Connection) (err error) {

	session, err := EstablishConnection(c.remoteAddr, c.remotePort, c.remoteUser)
	if err != nil {
		return
	}
	defer session.Close()

	// keyPath := "/tmp/" + c.newUser -> where to write user key in pod

	file, err := os.Create(c.newUser) //filename
	if err != nil {
		return 
	}

	_, err = io.Copy(file, strings.NewReader(c.publicKey))
	if err != nil {
		return 
	}

	file.Close()

	file, _ = os.Open(c.newUser)

	defer file.Close()

	stat, _ := file.Stat()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		hostIn, _ := session.StdinPipe()
		defer hostIn.Close()
		fmt.Fprintf(hostIn, "C0664 %d %s\n", stat.Size(), c.newUser+".pub") // file name in the remote host, s263084.pub
		io.Copy(hostIn, file)
		fmt.Fprint(hostIn, "\x00")
		wg.Done()
	}()

	var b bytes.Buffer
	session.Stdout = &b
	keyPath := HOME + c.remoteUser                                         // where to copy the publicKey in the remote server
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addstudent.sh " + c.newUser // scp copies pKey in remote server, addstudent.sh copies it in new user
	if err = session.Run(cmd); err != nil {
		return
	}

	fmt.Println(b.String())
	wg.Wait()

	return nil
}

func DeleteUser(c Connection) (err error) {

	session, err := EstablishConnection(c.remoteAddr, c.remotePort, c.remoteUser)
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	cmd := "pkill -KILL -u " + c.newUser + "; deluser --remove-home " + c.newUser
	if err = session.Run(cmd); err != nil {
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
	if !containsString(instance.Finalizers, FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	conn := Connection{
		remoteAddr: "10.244.3.164",
		remoteUser: "davide",
		remotePort: "22",
		publicKey:  instance.Spec.PublicKey,
		newUser:    instance.Spec.ID,
	}

	err = AddUser(conn)
	if err != nil {
		errLogger := log.WithValues("Error", err)
		errLogger.Error(err, "Error")
		// TODO re-reconcile, don't stop 
		// TODO initializer
	} else {
		reqLogger := log.WithValues("Student ID", instance.Spec.ID)
		reqLogger.Info("Created new student")
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

	conn := Connection{
		remoteAddr: "10.244.3.164",
		remoteUser: "davide",
		remotePort: "22",
		publicKey:  instance.Spec.PublicKey,
		newUser:    instance.Spec.ID,
	}

	// look for matching finalizers
	if containsString(instance.Finalizers, FINALIZER) {
		// if requested finalizer is present, we will handle the
		// deletion of external resource, i.e. a user account
		if err = DeleteUser(conn); err != nil {
			// if fail to delete the external dependency here, return with error
			// so that it can be retried
			return reconcile.Result{}, err
		}

		// remove our finalizer from the list and update it.
		instance.Finalizers = removeString(instance.Finalizers, FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}

		reqLogger := log.WithValues("Student ID", instance.Spec.ID)
		reqLogger.Info("Deleted student")

	}

	return reconcile.Result{}, nil
}

// TODO login on multiple machines (maybe watching the kubernetes label)
// TODO add label on CR to distinguish accessible machines
// TODO synchronize the access when a machine is created/deleted
// TODO quanti utenti connessi
// TODO handle all possible errors
// TODO refactor with class
// TODO add name and surname when registering
