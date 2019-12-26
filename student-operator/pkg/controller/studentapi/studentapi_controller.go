package studentapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	//"time"
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

// TODO add var such kvp here
var (
	log = logf.Log.WithName("controller_studentapi")
)


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

type Config struct {
	Remoteuser string `yaml:"remote-user"`
	Remoteport string `yaml:"remote-port"`
	Remoteaddr string `yaml:"remote-addr"`
	Roles 	   []string `yaml:"roles"`
}

// TODO add const such as kvp, move this block up
const (
	PRIVATE_KEY  string = "/home/davide/.ssh/id_rsa"
	BASTION      string = "bastion"
	BASTION_ADDR string = "130.192.225.74:22"
	HOME         string = "/home/"
	S_FINALIZER  string = "finalizers/student"
	C_FINALIZER  string = "finalizers/cmap"
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
		return nil, err
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
	defer session.Close()

	wG := sync.WaitGroup{}
	wG.Add(1)

	go func() {
		stdin, _ := session.StdinPipe()
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0664 %d %s\n", stat.Size(), c.newUser+".pub") // file name in the remote host, s263084.pub
		io.Copy(stdin, file)
		fmt.Fprint(stdin, "\x00")
		wG.Done()
	}()

	var b bytes.Buffer
	session.Stdout = &b
	keyPath := HOME + c.remoteUser                                             // where to copy the publicKey in the remote server
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addstudent.sh " + c.newUser // scp copies pKey in remote server, addstudent.sh copies it in new user
	if err = session.Run(cmd); err != nil {
		return
	}

	log.V(1).Info(fmt.Sprintf(b.String()))
	wG.Wait()

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

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return 
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	
	go func() {
	hostIn, _ := session.StdinPipe()
	defer hostIn.Close()
	fmt.Fprintf(hostIn, "C0664 %d %s\n", stat.Size(), c.newUser + ".pub") // file name in the remote host, s263084.pub
	io.Copy(hostIn, file)
	fmt.Fprint(hostIn, "\x00")
	wg.Done()
	}()
	

	var b bytes.Buffer
	session.Stdout = &b
	keyPath = HOME + c.remoteUser                                         // where to copy the publicKey in the remote server
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addstudent.sh " + c.newUser // scp copies pKey in remote server, addstudent.sh copies it in new user
	if err = session.Run(cmd); err != nil {
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

	// log for debugging
	log.V(1).Info(fmt.Sprintf(b.String()))

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
				if val == kvp.value && (len(e.MetaNew.GetFinalizers()) == len(e.MetaOld.GetFinalizers()) && e.MetaNew.GetDeletionTimestamp().IsZero()) {
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
			labels := e.MetaOld.GetLabels()
			if val, ok := labels[kvp.key]; ok {
				if val == kvp.value && !e.MetaNew.GetDeletionTimestamp().IsZero(){
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

	if !containsString(instance.Finalizers, C_FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, C_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	yml := instance.Data["config"]

	var config Config

	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		log.Error(err,err.Error())
	}	

	users := &netgroupv1.StudentAPIList{}
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
			// if user role match with at least one of the server
			// authorized role then AddUser()

			// sets desired state of the object
			if !containsString(user.Spec.Servers, config.Remoteaddr) {
				user.Spec.Servers = append(user.Spec.Servers, config.Remoteaddr)
				if err = r.client.Update(context.TODO(), &user); err != nil {
					return reconcile.Result{}, err
				}
			}
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  user.Info.PublicKey,
					newUser:    user.Info.ID,
				}

			if !containsString(user.Stat.Servers, config.Remoteaddr) {
				err = AddUser(conn)
				if err != nil {
					log.Error(err, err.Error())
					// TODO no reconcile immediately, but not update status
					
					//return reconcile.Result{RequeueAfter: time.Second * 20}, nil
				} else {
					// sets observed state of the user
					user.Stat.Servers = append(user.Stat.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), &user); err != nil {
						return reconcile.Result{}, err
					}
			    }
			} else {
				log.Info(fmt.Sprintf("User %s already present in %s",user.Info.ID, config.Remoteaddr))
				break
			}
				
			log.Info(fmt.Sprintf("User %s added to %s",user.Info.ID, config.Remoteaddr))
			break
			}
		}
	}

	log.Info(fmt.Sprintf("Server at %s correctly created",config.Remoteaddr))

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

	yml := instance.Data["config"]

	var config Config

	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		log.Error(err,err.Error())
	}	

	users := &netgroupv1.StudentAPIList{}

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

			if containsString(user.Spec.Servers, config.Remoteaddr) {
				user.Spec.Servers = removeString(user.Spec.Servers, config.Remoteaddr)
				if err = r.client.Update(context.TODO(), &user); err != nil {
					return reconcile.Result{}, err
				}
			}
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  user.Info.PublicKey,
					newUser:    user.Info.ID,
				}

				if containsString(user.Stat.Servers, config.Remoteaddr) {
					err = DeleteUser(conn)
					if err != nil {
						log.Error(err, err.Error())
						
						// TODO not reconcile immediately
						//return reconcile.Result{RequeueAfter: time.Second * 20}, nil
					} else {
						user.Stat.Servers = removeString(user.Stat.Servers, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), &user); err != nil {
							return reconcile.Result{}, err
						}
					}

				} else {
					log.Info(fmt.Sprintf("User %s already deleted from %s",user.Info.ID, config.Remoteaddr))
					break
				}

				log.Info(fmt.Sprintf("User %s deleted from %s",user.Info.ID, config.Remoteaddr))
				break
			}
		}
	}

	// looks for matching finalizers
	if containsString(instance.Finalizers, C_FINALIZER) {
		// remove our finalizer from the list and update it.
		instance.Finalizers = removeString(instance.Finalizers, C_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	log.Info(fmt.Sprintf("Server at %s correctly deleted",config.Remoteaddr))

	return reconcile.Result{}, nil
}
 
// Reconcile reads that state of the cluster for a StudentAPI object and makes changes based on the state read
// and what is in the StudentAPI.Info
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
	if !containsString(instance.Finalizers, S_FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, S_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	cmaps := &v1.ConfigMapList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string] string {"use": "StudentAPI"}),
	}
	err = r.client.List(context.TODO(), cmaps, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all cmaps
	for _, cmap := range cmaps.Items {
		yml := cmap.Data["config"]

		var config Config

		if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
			log.Error(err,err.Error())
		}	

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the server
			// authorized roles then AddUser()

			// sets desired state of the object
			if !containsString(instance.Spec.Servers, config.Remoteaddr) {
				instance.Spec.Servers = append(instance.Spec.Servers, config.Remoteaddr)
				if err = r.client.Update(context.TODO(), instance); err != nil {
					return reconcile.Result{}, err
				}
			}
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  instance.Info.PublicKey,
					newUser:    instance.Info.ID,
				}

			if !containsString(instance.Stat.Servers, config.Remoteaddr) {
				err = AddUser(conn)
				if err != nil {
					log.Error(err, err.Error())
					// TODO no reconcile immediately, but not update status
					
					//return reconcile.Result{RequeueAfter: time.Second * 20}, nil
				} else {
					// sets observed state of the user
					instance.Stat.Servers = append(instance.Stat.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
			    }
			} else {
				log.Info(fmt.Sprintf("User %s already present in %s", instance.Info.ID, config.Remoteaddr))
				break
			}
				
			log.Info(fmt.Sprintf("User %s added to %s", instance.Info.ID, config.Remoteaddr))
			break
			}
		}
	}

	reqLogger := log.WithValues("User ID", instance.Info.ID)
	reqLogger.Info("Correctly created new user")

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

	cmaps := &v1.ConfigMapList{}
	opts := []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string] string {"use": "StudentAPI"}),
	}
	err = r.client.List(context.TODO(), cmaps, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate over all cmaps
	for _, cmap := range cmaps.Items {
		yml := cmap.Data["config"]

		var config Config

		if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
			log.Error(err,err.Error())
		}	

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the server
			// authorized roles then delete user from that machine

			// sets desired state of the object
			if containsString(instance.Spec.Servers, config.Remoteaddr) {
				instance.Spec.Servers = removeString(instance.Spec.Servers, config.Remoteaddr)
				if err = r.client.Update(context.TODO(), instance); err != nil {
					return reconcile.Result{}, err
				}
			}
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  instance.Info.PublicKey,
					newUser:    instance.Info.ID,
				}

			if containsString(instance.Stat.Servers, config.Remoteaddr) {
				err = DeleteUser(conn)
				if err != nil {
					log.Error(err, err.Error())
					// TODO no reconcile immediately, but not update status
					
					//return reconcile.Result{RequeueAfter: time.Second * 20}, nil
				} else {
					// sets observed state of the user
					instance.Stat.Servers = removeString(instance.Stat.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
			    }
			} else {
				log.Info(fmt.Sprintf("User %s already deleted from %s", instance.Info.ID, config.Remoteaddr))
				break
			}
				
			log.Info(fmt.Sprintf("User %s deleted from %s", instance.Info.ID, config.Remoteaddr))
			break
			}
		}
	}

	// look for matching finalizers
	if containsString(instance.Finalizers, S_FINALIZER) {
		// remove our finalizer from the list and update it.
		instance.Finalizers = removeString(instance.Finalizers, S_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}
		

	reqLogger := log.WithValues("Student ID", instance.Info.ID)
	reqLogger.Info("Correctly deleted user")

	return reconcile.Result{}, nil
}

// TODO gestione concorrenza
// TODO quanti utenti connessi
// TODO handle all possible errors
// TODO refactor with class
// TODO add name and surname when registering
// TODO use Spec and Stat to see the running users
