package studentapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"io/ioutil"

	netgroupv1 "github.com/example-inc/memcached-operator/pkg/apis/netgroup/v1"
	"k8s.io/api/core/v1"
	"golang.org/x/crypto/ssh"
	"github.com/goccy/go-yaml"
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

// this function connects to the remote machine and creates a new user named with his studentID
func AddUser(c Connection) (err error) {

	session, err := EstablishConnection(c.remoteAddr, c.remotePort, c.remoteUser)
	if err != nil {
		return
	}
	defer session.Close()

	file, err := os.Create(c.newUser) //filename
	if err != nil {
		return
	}

	_, err = io.Copy(file, strings.NewReader(c.publicKey))
	if err != nil {
		return
	}

	file.Close()

	file, err = os.Open(c.newUser)
	if err != nil {
		return
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		stdin, _ := session.StdinPipe()
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0664 %d %s\n", stat.Size(), c.newUser+".pub") // file name in the remote host, s263084.pub
		io.Copy(stdin, file)
		fmt.Fprint(stdin, "\x00")
		wg.Done()
	}()

	var b bytes.Buffer
	session.Stdout = &b
	keyPath := HOME + c.remoteUser                                             // where to copy the publicKey in the remote server
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addstudent.sh " + c.newUser // scp copies pKey in remote server, addstudent.sh copies it in new user
	if err = session.Run(cmd); err != nil {
		return
	}

	log.Info(fmt.Sprintf(b.String()))
	wg.Wait()

	return nil
}

func DeleteUser(c Connection) (err error) {

	session, err := EstablishConnection(c.remoteAddr, c.remotePort, c.remoteUser)
	if err != nil {
		return
	}
	defer session.Close()

	// StdinPipe for commands
	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}

	err = session.Shell()
	if err != nil {
		return
	}

	var b bytes.Buffer
	session.Stdout = &b

	commands := []string { 
		"pkill -KILL -u " + c.newUser,
		"sudo deluser --remove-home " + c.newUser,
		"rm " + c.newUser + ".pub",
		"exit",
	}

	for _, cmd := range commands {
		_, err = fmt.Fprintf(stdin, "%s\n", cmd)
		if err != nil {
			return
		}
	}

	err = session.Wait()
	if err != nil {
		return
	}

	log.Info(fmt.Sprintf(b.String()))

	return nil

}

type keyValuePair struct {
	key   string
	value string
}

var kvp keyValuePair

// Add creates two new StudentAPI Controllers and adds them to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconcilerCreate(mgr), newReconcilerDelete(mgr), newReconcilerCreateServer(mgr), newReconcilerDeleteServer(mgr))
}

// newReconcilerCreate returns a new reconcile.Reconciler
func newReconcilerCreate(mgr manager.Manager) reconcile.Reconciler {
	return &CreateReconcileStudentAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerDelete returns a new reconcile.Reconciler
func newReconcilerDelete(mgr manager.Manager) reconcile.Reconciler {
	return &DeleteReconcileStudentAPI{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerServer returns a new reconcile.Reconciler
func newReconcilerCreateServer(mgr manager.Manager) reconcile.Reconciler {
	return &ServerCreateReconcile{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// newReconcilerServer returns a new reconcile.Reconciler
func newReconcilerDeleteServer(mgr manager.Manager) reconcile.Reconciler {
	return &ServerDeleteReconcile{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds new Controllers to mgr
func add(mgr manager.Manager, rCreate reconcile.Reconciler, rDelete reconcile.Reconciler, rCreateServer reconcile.Reconciler, rDeleteServer reconcile.Reconciler) error {
	// Create a new controller for Create event
	c_create, err := controller.New("create-controller", mgr, controller.Options{Reconciler: rCreate})
	if err != nil {
		return err
	}

	src_StudentAPI := &source.Kind{Type: &netgroupv1.StudentAPI{}}

	h := &handler.EnqueueRequestForObject{}

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
	err = c_create.Watch(src_StudentAPI, h, pred_create)
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
			return !e.MetaNew.GetDeletionTimestamp().IsZero()
		},
	}

	// Watch for changes to primary resource StudentAPI
	err = c_delete.Watch(src_StudentAPI, h, pred_delete)
	if err != nil {
		return err
	}

	c_serv, err := controller.New("server-create-controller", mgr, controller.Options{Reconciler: rCreateServer})
	if err != nil {
		return err
	}

	src_serv := &source.Kind{Type: &v1.ConfigMap{}}

	kvp := keyValuePair{ 
		key:   "use",
		value: "StudentAPI",
	}

	pred_serv_c := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			labels := e.Meta.GetLabels()
			if val, ok := labels[kvp.key]; ok {
				if val == kvp.value {
				return true
			}
		}
		return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			labels := e.MetaOld.GetLabels()
			if val, ok := labels[kvp.key]; ok {
				if val == kvp.value {
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

	c_servDelete, err := controller.New("server-delete-controller", mgr, controller.Options{Reconciler: rDeleteServer})
	if err != nil {
		return err
	}

	pred_serv_d := predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			labels := e.Meta.GetLabels()
			if val, ok := labels[kvp.key]; ok {
				if val == kvp.value {
				return true
			}
		}
		return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
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

type ServerCreateReconcile struct {
	client client.Client
	scheme *runtime.Scheme
}

type ServerDeleteReconcile struct {
	client client.Client
	scheme *runtime.Scheme
}

func (r *ServerCreateReconcile) Reconcile (request reconcile.Request) (reconcile.Result, error) {
	// Fetch the StudentAPI instance
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

	yml := instance.Data["config"]

    var config struct {
		Remoteuser string `yaml:"remote-user"`
		Remoteport string `yaml:"remote-port"`
		Remoteaddr string `yaml:"remote-addr"`
		Roles 	   []string `yaml:"roles"`
	}

	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		log.Error(err,err.Error())
	}	

	users := &netgroupv1.StudentAPIList{}
	opts := []client.ListOption{
		//client.MatchingFieldsSelector{Selector: fields.},
		client.InNamespace(instance.Namespace),
	}
	err = r.client.List(context.TODO(), users, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all objects
	for _, user := range users.Items {
		// iterate over roles of one object
		for _, role := range user.Spec.Roles {
			// if user role match with at least one of the server
			// authorized role then AddUser()

			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  user.Spec.PublicKey,
					newUser:    user.Spec.ID,
				}

				err = AddUser(conn)
				if err != nil {
					log.Error(err, err.Error())
					// instead of return immediately try to set a label with "completed" and continue to add students
					// maybe better on student add
					return reconcile.Result{RequeueAfter: time.Second * 20}, nil
				}

				log.Info(fmt.Sprintf("User %s added to %s",user.Spec.ID, config.Remoteaddr))
				break
			}
		}
	}

	return reconcile.Result{}, nil
}

func (r *ServerDeleteReconcile) Reconcile (request reconcile.Request) (reconcile.Result, error) {
	// Fetch the StudentAPI instance
	instance := &v1.ConfigMap{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// We'll ignore not found errors since the object could
		// be already deleted

		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Deleted ConfigMap")

	return reconcile.Result{}, nil
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
		/*
		errLogger := log.WithValues("Error", err)
		errLogger.Error(err, "Error")*/
		log.Error(err, err.Error())
		return reconcile.Result{RequeueAfter: time.Second * 20}, nil
	}

	reqLogger := log.WithValues("Student ID", instance.Spec.ID)
	reqLogger.Info("Created new student")

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
	}

	reqLogger := log.WithValues("Student ID", instance.Spec.ID)
	reqLogger.Info("Deleted student")

	return reconcile.Result{}, nil
}

// TODO login on multiple machines (maybe watching the kubernetes label)
// TODO add label on CR to distinguish accessible machines
// TODO synchronize the access when a machine is created/deleted
// TODO quanti utenti connessi
// TODO handle all possible errors
// TODO refactor with class
// TODO add name and surname when registering
// TODO use Spec and Status to see the running users
