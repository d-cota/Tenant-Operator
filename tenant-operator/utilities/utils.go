package utilities

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"golang.org/x/crypto/ssh"
)

/* --- GLOBAL VARIABLES AND CONSTANTS --- */

// KeyValuePair contains key and value
// for Host ConfigMap labels
type KeyValuePair struct {
	Key   string
	Value string
}

// Connection is a struct to
// pass user infos to the functions
type Connection struct {
	RemoteAddr string
	RemotePort string
	RemoteUser string
	PublicKey  string
	NewUser    string
}

// Config is used to parse the yaml
// inside the ConfigMap
type Config struct {
	Remoteuser string   `yaml:"remote-user"`
	Remoteport string   `yaml:"remote-port"`
	Remoteaddr string   `yaml:"remote-addr"`
	Roles      []string `yaml:"roles"`
}

const (
	PRIVATE_KEY string = "/etc/secret-volume/ssh-privatekey"
	HOME        string = "/home/"
	S_FINALIZER string = "finalizers/tenant"
	C_FINALIZER string = "finalizers/cmap"
)

var (
	BASTION         = os.Getenv("BASTION")
	BASTION_ADDR    = os.Getenv("BASTION_ADDR")
	MAIL_FROM       = os.Getenv("MAIL_FROM")
	MAIL_PASS       = os.Getenv("MAIL_PASS")
	POD_RELEASE     = os.Getenv("POD_RELEASE")
	SERVICE_RELEASE = os.Getenv("SERVICE_RELEASE")
)

/* --- GENERIC FUNCTIONS --- */

// containsString check string from a slice of strings.
// returns: true if s is present, false otherwise
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString remove string from a slice of strings.
// returns: modified slice
func RemoveString(slice []string, s string) (result []string) {
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

	session, err := EstablishConnection(c.RemoteAddr, c.RemotePort, c.RemoteUser)
	if err != nil {
		return
	}
	// close the session when AddUser returns
	defer session.Close()

	// path where to write user key in the container
	keyPath := "/tmp/" + c.NewUser + ".pub"

	// create the key file in the container
	file, err := os.Create(keyPath)
	if err != nil {
		return
	}

	// fill the file with the user public key
	_, err = io.Copy(file, strings.NewReader(c.PublicKey))
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
		fmt.Fprintf(stdin, "C0664 %d %s\n", stat.Size(), c.NewUser+".pub") // file name in the remote host, s263084.pub
		io.Copy(stdin, file)
		fmt.Fprint(stdin, "\x00")
		wG.Done()
	}()

	var b bytes.Buffer
	session.Stdout = &b
	// path where to copy the publicKey in the remote server
	keyPath = HOME + c.RemoteUser
	// scp copies the pub key in remote server, addstudent.sh copies it in new user
	cmd := "/usr/bin/scp -t " + keyPath + ";sudo ./addtenant.sh " + c.NewUser
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

	session, err := EstablishConnection(c.RemoteAddr, c.RemotePort, c.RemoteUser)
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
	commands := []string{
		"pkill -KILL -u " + c.NewUser,
		"sudo deluser --remove-home " + c.NewUser,
		"rm " + c.NewUser + ".pub",
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
func SendEmail(to string, machines []string) (err error) {
	// Choose auth method and set it up
	auth := smtp.PlainAuth("", MAIL_FROM, MAIL_PASS, "smtp.gmail.com")

	// Here we do it all: connect to our server, set up a message and send it
	msg := "From:" + MAIL_FROM +
		"To:" + to + "\n" +
		"Subject: Welcome to the Cloud Computing Lab\r\n" +
		"\r\n" +
		"Hi, you're just been added to the Cloud Computing Lab.\r\n" +
		"An account has been created for you in the following machines:\n" +
		strings.Join(machines, "\n") +
		"\nThe public key that you subscribed has already been added to the server, from now on you can connect from remote to the machine via ssh." +
		"\nBest regards,\n\r Cloud Computing team"
	err = smtp.SendMail("smtp.gmail.com:587", auth, MAIL_FROM, []string{to}, []byte(msg))
	if err != nil {
		return
	}

	return nil
}

// GenerateVPNCert launch a sh file in a pod where a new .ovpn certificate is generated,
// then creates the corresponding secret
// returns: nil if success, err if error
func GenerateVPNCert(user string, pod_name string, service_ip string, log logr.Logger) (err error) {
	// launch the sh file in the container
	cmd := exec.Command("./root/kubectl", "exec", "-it", pod_name, "-c", "openvpn", "/etc/openvpn/setup/newClientCert.sh", user, service_ip)
	_, err = cmd.Output()
	if err != nil {
		return
	}

	// redirect the cat output in the os stdout, then save the certificate in a file in the operator container
	cmd = exec.Command("./root/kubectl", "exec", "-it", pod_name, "-c", "openvpn", "cat", "/etc/openvpn/certs/pki/"+user+".ovpn")
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	err = cmd.Run()
	if err != nil {
		log.Info(fmt.Sprintf("%s: %s", err, stderr.String()))
		return
	}

	file, err := os.Create("/root/" + user + "-ovpn")
	if err != nil {
		return
	}
	_, err = io.Copy(file, strings.NewReader(stdout.String()))
	if err != nil {
		return
	}

	file.Close()

	// create the corresponding secret
	cmd = exec.Command("./root/kubectl", "create", "secret", "generic", user+"-ovpn", "--from-file=/root/"+user+"-ovpn")
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		log.Info(fmt.Sprintf("%s: %s", err, stderr.String()))
		return
	}

	return nil
}
