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
	"os/exec"
	"net/smtp"

	netgroupv1 "github.com/example-inc/memcached-operator/pkg/apis/netgroup/v1"
	"k8s.io/api/core/v1"
	"golang.org/x/crypto/ssh"
	"github.com/goccy/go-yaml"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	t "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/* --- GLOBAL VARIABLES AND CONSTANTS --- */

// keyValuePair contains key and value
// for Server ConfigMap labels
type keyValuePair struct {
	key   string
	value string
}

// Connection is a struct to
// pass user infos to the functions
type Connection struct {
	remoteAddr string
	remotePort string
	remoteUser string
	publicKey  string
	newUser    string
}

// Config is used to parse the yaml
// inside the ConfigMap
type Config struct {
	Remoteuser string `yaml:"remote-user"`
	Remoteport string `yaml:"remote-port"`
	Remoteaddr string `yaml:"remote-addr"`
	Roles 	   []string `yaml:"roles"`
}


var (
	// a logger for each controller
	c_log = logf.Log.WithName("controller_create_studentapi")
	d_log = logf.Log.WithName("controller_delete_studentapi")
	cm_log = logf.Log.WithName("controller_create_server")
	dm_log = logf.Log.WithName("controller_delete_server")
	kvp keyValuePair
)

const (
	PRIVATE_KEY  string = "/etc/secret-volume/ssh-privatekey"
	BASTION      string = "bastion"
	BASTION_ADDR string = "130.192.225.74:22"
	HOME         string = "/home/"
	S_FINALIZER  string = "finalizers/student"
	C_FINALIZER  string = "finalizers/cmap"
	FROM 		 string = "coursecloudcomputing@gmail.com"
)

/* --- GENERIC FUNCTIONS --- */

// containsString check string from a slice of strings.
// returns: true if s is present, false otherwise
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString remove string from a slice of strings.
// returns: modified slice
func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// EstablishConnection prepare the fields to ssh jump through the bastion
// returns: (Session, nil) if success; (nil, err) if an error occurs
func EstablishConnection(remoteAddr string, remotePort string, remoteUser string) (*ssh.Session, error) {
	key, err := ioutil.ReadFile(PRIVATE_KEY) // path to bastion private key authentication
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	// this is similar to a ssh config file, here it is specified bastion addr
	// and the authentication method
	config := &ssh.ClientConfig{
		User: BASTION,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// starts dialing with the bastion
	bClient, err := ssh.Dial("tcp", BASTION_ADDR, config)
	if err != nil {
		return nil, err
	}

	rAddr := remoteAddr + ":" + remotePort
	conn, err := bClient.Dial("tcp", rAddr) // start dialing with remote server
	if err != nil {
		return nil, err
	}

	// here it is specified the user name and his authentication method (public key)
	config = &ssh.ClientConfig{
		User: remoteUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// start a new connection to the client
	ncc, chans, reqs, err := ssh.NewClientConn(conn, rAddr, config)
	if err != nil {
		return nil, err
	}

	sClient := ssh.NewClient(ncc, chans, reqs)

	return sClient.NewSession()
}

// AddUser connects to the remote machine and creates a new user named with his studentID
// returns: nil if success, err if error
func AddUser(c Connection, log logr.Logger) (err error) {

	session, err := EstablishConnection(c.remoteAddr, c.remotePort, c.remoteUser)
	if err != nil {
		return
	}
	// close the session when AddUser returns
	defer session.Close()

	// path where to write user key in the container
	keyPath := "/tmp/" + c.newUser + ".pub" 

	// create the key file in the container
	file, err := os.Create(keyPath) 
	if err != nil {
		return
	}

	// fill the file with the user public key
	_, err = io.Copy(file, strings.NewReader(c.publicKey))
	if err != nil {
		return 
	}

	file.Close()

	// it is needed to close and reopen the file otherwise it doesn't work
	file, err = os.Open(keyPath)
	if err != nil {
		return
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return
	}
	defer session.Close()

	// synchronization
	wG := sync.WaitGroup{}
	wG.Add(1)

	go func() {
		stdin, _ := session.StdinPipe()
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0664 %d %s\n", stat.Size(), c.newUser + ".pub") // file name in the remote host, s263084.pub
		io.Copy(stdin, file)
		fmt.Fprint(stdin, "\x00")
		wG.Done()
	}()

	var b bytes.Buffer
	session.Stdout = &b
	// path where to copy the publicKey in the remote server
	keyPath = HOME + c.remoteUser    
	// scp copies the pub key in remote server, addstudent.sh copies it in new user                                        
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addstudent.sh " + c.newUser 
	if err = session.Run(cmd); err != nil {
		return
	}

	// verbosity level V(1) only used in debugging
	log.V(1).Info(fmt.Sprintf(b.String()))
	wG.Wait()

	return nil
}

// DeleteUser delete an user from a remote machines via ssh
// returns: nil if success, err if error
func DeleteUser(c Connection, log logr.Logger) (err error) {

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

	// starts the remote shell
	err = session.Shell()
	if err != nil {
		return
	}

	var b bytes.Buffer
	session.Stdout = &b

	// kick off the user if logged, erase the user account from the machine
	// delete his pub key from the server
	commands := []string { 
		"pkill -KILL -u " + c.newUser,
		"sudo deluser --remove-home " + c.newUser,
		"rm " + c.newUser + ".pub",
		"exit",
	}

	// launch the commands in the remote shell
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

// sendEmail sends an email with the list of user authorized machines
// returns: nil if success, err if error
func sendEmail(to string, machines []string) (err error){
	// Choose auth method and set it up
	auth := smtp.PlainAuth("", FROM, "verysecretpass", "smtp.gmail.com")

	// Here we do it all: connect to our server, set up a message and send it
	msg := "From:" + FROM +
			"To:" + to + "\n" +
			"Subject: Welcome to the Cloud Computing Lab\r\n" +
			"\r\n" +
			"Hi, you're just been added to the Cloud Computing Lab.\r\n" + 
			"An account has been created for you in the following machines:\n" + 
			strings.Join(machines, "\n") +
			"\nThe public key that you subscribed has already been added to the server, from now on you can connect from remote to the machine via ssh." +
			"\nBest regards,\n\r Cloud Computing team"
	err = smtp.SendMail("smtp.gmail.com:587", auth, FROM, []string{to}, []byte(msg))
	if err != nil {
		return 
	}

	return nil
}

// GenerateVPNCert launch a sh file in a pod where a new .ovpn certificate is generated, 
// then creates the corresponding secret
// returns: nil if success, err if error
func GenerateVPNCert(user string, pod_name string, service_ip string) (err error) {
	// launch the sh file in the container
	cmd := exec.Command("./root/kubectl", "exec", "-it", pod_name, "-c", "openvpn", "/etc/openvpn/setup/newClientCert.sh", user, service_ip)
	_, err = cmd.Output()
	if err != nil {
		return
	}

	// redirect the cat output in the os stdout, then save the certificate in a file in the operator container
	cmd = exec.Command("./root/kubectl", "exec", "-it", pod_name, "-c", "openvpn", "cat", "/etc/openvpn/certs/pki/" + user + ".ovpn")
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err = cmd.Run()
	if err != nil {
		c_log.Info(fmt.Sprintf("%s: %s",err, stderr.String()))
		return
	}

	file, err := os.Create("/root/" + user + ".ovpn") 
	if err != nil {
		return
	}
	_, err = io.Copy(file, strings.NewReader(stdout.String()))
	if err != nil {
		return 
	}

	file.Close()

	// create the corresponding secret
	cmd = exec.Command("./root/kubectl", "create", "secret", "generic", user + ".ovpn", "--from-file=/root/" + user + ".ovpn")
	err = cmd.Run()
	if err != nil {
		return
	}

	return nil
}

/* --- CONTROLLER --- */
// Add creates four new StudentAPI Controllers and adds them to the Manager. The Manager will set fields on the Controller
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
// 4 controllers: Create a Student, Delete a Student, Create a Configmap Server, Delete a Configmap Server
func add(mgr manager.Manager, rCreate reconcile.Reconciler, rDelete reconcile.Reconciler, rCreateServer reconcile.Reconciler, rDeleteServer reconcile.Reconciler) error {
	// Create a new controller for Create event
	c_create, err := controller.New("create-controller", mgr, controller.Options{Reconciler: rCreate})
	if err != nil {
		return err
	}

	// watch StudentAPI object
	src_StudentAPI := &source.Kind{Type: &netgroupv1.StudentAPI{}}

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
			// since a finalizer is present, reacts when the DeletionTimestamp is not zero
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

	// watch the ConfigMap
	src_serv := &source.Kind{Type: &v1.ConfigMap{}}

	kvp := keyValuePair{ 
		key:   "use",
		value: "StudentAPI",
	}

	// events to watch, but reacts only to the ConfigMap labeled "use"="StudentAPI"
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
				// react only when the finalizer is not updated
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

/* --- RECONCILERS --- */

// Reconcile reconciles a Server ConfigMap when it is created. It looks for all the student
// with the matching Role field and it adds them to the machine
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

	// add ConfigMap finalizer
	if !containsString(instance.Finalizers, C_FINALIZER) {
		instance.Finalizers = append(instance.Finalizers, C_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	yml := instance.Data["config"]

	var config Config

	// read the yaml in the ConfigMap
	if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
		cm_log.Error(err,err.Error())
	}	

	// list all the student CRs in the namespace
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
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				// sets desired state of the object
				if !containsString(user.Spec.Servers, config.Remoteaddr) {
					user.Spec.Servers = append(user.Spec.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), &user); err != nil {
						return reconcile.Result{}, err
					}
				}
				
				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  user.Info.PublicKey,
					newUser:    user.Info.ID,
				}

				// if the user is not present in the machine (Spec is not empty, Stat it is), then add him 
				if !containsString(user.Stat.Servers, config.Remoteaddr) {
					err = AddUser(conn, cm_log)
					if err != nil {
						// if any error occurs, reconcile after 30 sec
						cm_log.Error(err, err.Error())
					
						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						user.Stat.Servers = append(user.Stat.Servers, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), &user); err != nil {
							return reconcile.Result{}, err
						}
						cm_log.Info(fmt.Sprintf("User %s added to %s",user.Info.ID, config.Remoteaddr))
						break
			    	}
				} else {
					cm_log.Info(fmt.Sprintf("User %s already present in %s",user.Info.ID, config.Remoteaddr))
					break
				}
			}
		}
	}

	cm_log.Info(fmt.Sprintf("Server at %s correctly created",config.Remoteaddr))

	return reconcile.Result{}, nil
}

// Reconcile reconcile a ConfigMap Server at the deletion. It deletes all accounts in the machine
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
		d_log.Error(err,err.Error())
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
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {

				if containsString(user.Spec.Servers, config.Remoteaddr) {
					// delete Spec server address entry in the user CR
					user.Spec.Servers = removeString(user.Spec.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), &user); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  user.Info.PublicKey,
					newUser:    user.Info.ID,
				}

				if containsString(user.Stat.Servers, config.Remoteaddr) {
					err = DeleteUser(conn, dm_log)
					if err != nil {
						dm_log.Error(err, err.Error())
						
						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// delete Stat server address entry in the user CR
						user.Stat.Servers = removeString(user.Stat.Servers, config.Remoteaddr)
						if err = r.client.Update(context.TODO(), &user); err != nil {
							return reconcile.Result{}, err
						}
						dm_log.Info(fmt.Sprintf("User %s deleted from %s",user.Info.ID, config.Remoteaddr))
						break
					}

				} else {
					dm_log.Info(fmt.Sprintf("User %s already deleted from %s",user.Info.ID, config.Remoteaddr))
					break
				}
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

	dm_log.Info(fmt.Sprintf("Server at %s correctly deleted",config.Remoteaddr))

	return reconcile.Result{}, nil
}
 
// Reconcile reads that state of the cluster for a StudentAPI object and makes changes based on the state read
// and what is in the StudentAPI.Info
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
// Reconcile reconciles a StudentAPI object when it is created. It adds the student to
// all the authorized machines watching at Role field.
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

	// look for vpn pod
	pods := &v1.PodList{}
	opts := []client.ListOption{
		client.MatchingLabels(map[string] string {"app": "openvpn", "release":"oldfashioned-ocelot"}),
	}
	err = r.client.List(context.TODO(), pods, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	var pod_name string
	pod_name = pods.Items[0].Name

	// look for vpn service
	svc := &v1.ServiceList{}
	opts = []client.ListOption{
		client.MatchingLabels(map[string] string {"app": "openvpn", "release":"rude-hyena"}),
	}
	err = r.client.List(context.TODO(), svc, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	var service_ip string
	service_ip = svc.Items[0].Status.LoadBalancer.Ingress[0].IP

	// list all Server ConfigMap
	cmaps := &v1.ConfigMapList{}
	opts = []client.ListOption{
		client.InNamespace(instance.Namespace),
		client.MatchingLabels(map[string] string {"use": "StudentAPI"}),
	}
	err = r.client.List(context.TODO(), cmaps, opts...)
	if err != nil {
		return reconcile.Result{}, err
	}

	// list of user authorized servers 
	var machines []string
	// iterate over all cmaps
	for _, cmap := range cmaps.Items {
		yml := cmap.Data["config"]

		var config Config

		if err = yaml.Unmarshal([]byte(yml), &config); err != nil {
			c_log.Error(err,err.Error())
		}	

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the server
			// authorized roles then AddUser()
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				// sets desired state of the object
				if !containsString(instance.Spec.Servers, config.Remoteaddr) {
					instance.Spec.Servers = append(instance.Spec.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  instance.Info.PublicKey,
					newUser:    instance.Info.ID,
				}

				if !containsString(instance.Stat.Servers, config.Remoteaddr) {
					err = AddUser(conn, c_log)
					if err != nil {
						c_log.Error(err, err.Error())
					
						return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						instance.Stat.Servers = append(instance.Stat.Servers, config.Remoteaddr)
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

	// generate the user VPN certificate
	err = GenerateVPNCert(instance.Info.ID, pod_name, service_ip)
	if err != nil {
		c_log.Error(err,err.Error())
	}

	// send an email with instructions
	err = sendEmail(instance.Info.Email, machines)
	if err != nil {
		c_log.Error(err,err.Error())
	}

	reqLogger := c_log.WithValues("User ID", instance.Info.ID)
	reqLogger.Info("Correctly created new user")

	return reconcile.Result{}, nil
}

// Reconcile reconcile a StudentAPI object when it is deleted. It handles the account deletion
// from servers and revocates the ovpn certificate 
func (r *DeleteReconcileStudentAPI) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the StudentAPI instance
	instance := &netgroupv1.StudentAPI{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		// We'll ignore not found errors since the object could
		// be already deleted

		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// look for Server ConfigMap
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
			d_log.Error(err,err.Error())
		}	

		// iterate over roles of the user
		for _, role := range instance.Info.Roles {
			// if user role match with at least one of the server
			// authorized roles then delete user from that machine
			
			// iterate over ConfigMap roles
			if containsString(config.Roles, role) {
				// sets desired state of the object
				if containsString(instance.Spec.Servers, config.Remoteaddr) {
					instance.Spec.Servers = removeString(instance.Spec.Servers, config.Remoteaddr)
					if err = r.client.Update(context.TODO(), instance); err != nil {
						return reconcile.Result{}, err
					}
				}

				conn := Connection{
					remoteAddr: config.Remoteaddr,
					remoteUser: config.Remoteuser,
					remotePort: config.Remoteport,
					publicKey:  instance.Info.PublicKey,
					newUser:    instance.Info.ID,
				}

				if containsString(instance.Stat.Servers, config.Remoteaddr) {
					err = DeleteUser(conn, d_log)
					if err != nil {
						d_log.Error(err, err.Error())
					
						 return reconcile.Result{RequeueAfter: time.Second * 30}, nil
					} else {
						// sets observed state of the user
						instance.Stat.Servers = removeString(instance.Stat.Servers, config.Remoteaddr)
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

	// name of the secret containing the certificate to be deleted
	name := t.NamespacedName{Namespace: "dcota-ns1", Name: instance.Info.ID + ".ovpn"}
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
	if containsString(instance.Finalizers, S_FINALIZER) {
		// remove our finalizer from the list and update it.
		instance.Finalizers = removeString(instance.Finalizers, S_FINALIZER)
		if err = r.client.Update(context.TODO(), instance); err != nil {
			return reconcile.Result{}, err
		}
	}
		
	reqLogger := d_log.WithValues("Student ID", instance.Info.ID)
	reqLogger.Info("Correctly deleted user")

	return reconcile.Result{}, nil
}