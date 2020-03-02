# Tenant-as-a-Service

# Description

This repo hosts the Tenant-as-a-Service project exploiting the Kubernetes functionalities.
TenantOperator provides a reconcile logic in the lifecycle of a Custom Resource Tenant. The operator comes with 4 Controllers reacting to Create/Delete event for a Tenant CR as well as a ConfigMap describing any remote machine in which the user is supposed to be connected into.
The TenantOperator has been built using [Operator-SDK](https://github.com/operator-framework/operator-sdk). The structure of this documentation is divided into a section dedicated to common users and a section dedicated to developers.

## Installation

Follow the steps in the [Operator-SDK installation guide](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to learn how to install the Operator SDK CLI tool.

## Project Layout

Project scaffolding is explained [here](https://github.com/operator-framework/operator-sdk/blob/master/doc/project_layout.md).

## Prerequisites

- [git](https://git-scm.com/downloads)
- [go](https://golang.org/dl/) version v1.13+.
- [docker](https://docs.docker.com/install/) version 17.03+.
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) version v1.12.0+.
- Access to a Kubernetes v1.12.0+ cluster.

# Users
Common users that desire to use the TenantOperator only needs to create the Tenant CRD inside their cluster and deploy the operator. TenantOperator will begin immediately to monitor the Custom Resources.

## Usage

### Bastion
TenantOperator provides a way to perform an ssh-jump from bastion to another host. You need a valid ssh key to connect to the bastion and create a Kubernetes secret with the mentioned key.

### Mailing list
TenantOperator offers a method to report to the end user which hosts he has granted the access. You need a valid gmail account to expolit this functionality and create a Kubernetes secret with the related password. An appropriate secret can be obtained like this.
```sh
kubectl create secret generic <gmail-secret> --from-literal=<gmail-key-secret>='verysecretpass'
```

### Customize deployment
Edit the operator deployment manifest at [tenant-operator/deploy/operator.yaml](tenant-operator/deploy/operator.yaml). Below  are shown the lines that you need to modify in the yaml.

```yaml
[...]
        secret:
	# Replace this with the Kubernetes secret key name
          secretName: <bastion-ssh-key-secret>
[...]
	# Replace this with the bastion username
              value: <bastionusername>
            - name: BASTION_ADDR
	# Replace this with the bastion address and port
              value: <address>:<port>
            - name: MAIL_FROM
	# Replace this with your gmail account
              value: <mail>@gmail.com
            - name: MAIL_PASS
	# Replace this with your gmail password Kubernetes secret
              valueFrom:
                secretKeyRef:
                  name: <gmail-secret>
                  key: <gmail-key-secret>

```
### Create and deploy the operator
Open a Linux shell in the root folder of this project and type the following commands:
```sh
# Before launching these commands move in the tenant-operator folder.
# Setup Service Account
$ kubectl create -f deploy/service_account.yaml
# Setup RBAC
$ kubectl create -f deploy/role.yaml
$ kubectl create -f deploy/role_binding.yaml
# Setup the CRD
$ kubectl create -f deploy/crds/netgroup.com_tenants_crd.yaml
# Deploy the operator
$ kubectl create -f deploy/operator.yaml

# Create a Tenant CR
$ kubectl create -f examples/sampleuser_cr.yaml
# Create a Host ConfigMap
$ kubectl create -f examples/samplehost.yaml

# Verify that a pod is created
$ kubectl get pod

# Test the new Resource Type
$ kubectl describe tenants sampleuser
```

### Delete the resources
```sh
# Delete the CR
$ kubectl delete -f examples/sampleuser_cr.yaml

# Delete the host ConfigMap
$ kubectl delete -f examples/samplehost.yaml

# Delete the deployment
$ kubectl delete -f deploy/operator.yaml

# Delete the RBAC
$ kubectl delete -f deploy/role.yaml
$ kubectl delete -f deploy/role_binding.yaml
$ kubectl delete -f deploy/service_account.yaml

# Delete the CRD
$ kubectl delete -f deploy/crds/netgroup.com_tenants_crd.yaml
```

# Developers
Developers must follow the same steps presented in the 'Common User' section. A way to modify the operator is explained below.

## Usage

### Modify the operator
The go-lang code for the general purpose functions is at [tenant-operator/pkg/controller/tenant/tenant_controller.go](tenant-operator/pkg/controller/tenant/tenant_controller.go) for what concerns the Tenants handling, while the controllers monitoring the hosts are at [tenant-operator/pkg/controller/host/host_controller.go](tenant-operator/pkg/controller/host/host_controller.go). Each time you modify the code you have to re-build the operator and push the corresponding docker image. Then, you have to modify the operator deployment [tenant-operator/deploy/operator.yaml](tenant-operator/deploy/operator.yaml) changing the container image field with the newly built image.
```sh 
# Change <user> with your DockerHub username, a version can be added with :v<x.y>
# replace x and y with your version number
$ operator-sdk build <user>/tenant-operator

# Push the image, you need a docker.io account
$ docker push <user>/tenant-operator
```
Now replace the image field in the deploy/operator.yaml with your new image version:
```yaml
      containers:
        - name: tenant-operator
          # Replace this with the built image name
          image: docker.io/dcota1/tenant-operator:latest
```
### Modify the CRD

In order to modify the Tenant CRD you have to modify the code in [pkg/apis/netgroup/v1/tenant_types.go](pkg/apis/netgroup/v1/tenant_types.go). This will build a new Schema for the Kubernetes API to access the newly created.
Each time you change that file you have to run the following commands to regenerate the CRD yaml file and to rebuild the API schema:
```sh
$ operator-sdk generate k8s

$ operator-sdk generate openapis
```
