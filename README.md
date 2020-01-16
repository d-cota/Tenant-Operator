# Student-as-a-Service

# Description

This repo hosts the Student-as-a-Service project for the Cloud Computing course (at the Politecnico di Torino) exploiting the Kubernetes functionalities.
StudentOperator provides a reconcile logic in the lifecycle of a Custom Resource Student. The operator comes with 4 Controllers reacting to Create/Delete event for a Student CR as well as a ConfigMap describing the remote hosts.
The Operator has been built using [Operator-SDK](https://github.com/operator-framework/operator-sdk).

# Prerequisites

- [git](https://git-scm.com/downloads)
- [go](https://golang.org/dl/) version v1.13+.
- [mercurial](https://www.mercurial-scm.org/downloads) version 3.9+
- [docker](https://docs.docker.com/install/) version 17.03+.
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) version v1.12.0+.
- Access to a Kubernetes v1.12.0+ cluster.
# Installation

Follow the steps in the [Operator-SDK installation guide](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/install-operator-sdk.md) to learn how to install the Operator SDK CLI tool.

# Project Layout

Project scaffolding is explained [here](https://github.com/operator-framework/operator-sdk/blob/master/doc/project_layout.md).

# Usage
## Create and deploy the operator
```sh
# Before launching these commands move in the student-operator folder.
# Setup Service Account
$ kubectl create -f deploy/service_account.yaml
# Setup RBAC
$ kubectl create -f deploy/role.yaml
$ kubectl create -f deploy/role_binding.yaml
# Setup the CRD
$ kubectl create -f deploy/crds/netgroup.com_studentapis_crd.yaml
# Deploy the operator
$ kubectl create -f deploy/operator.yaml

# Create a Student CR
$ kubectl create -f deploy/crds/davidecota.yaml
# Create a Host ConfigMap
$ kubectl create -f utilities/my-server.yaml

# Verify that a pod is created
$ kubectl get pod

# Test the new Resource Type
$ kubectl describe studentapis davidecota

# Get the ovpn secret
$ kubectl get secret s263084-ovpn -o wide
```

## Modify the operator
The go-lang code for the general purpose functions is at [student-operator/pkg/controller/studentapi/studentapi_controller.go](student-operator/pkg/controller/studentapi/studentapi_controller.go). Each time you modify the code you have to re-build the operator and push the corresponding docker image. Then, you have to modify the operator deployment [student-operator/deploy/operator.yaml](https://github.com/netgroup-polito/StudentOperator/blob/master/student-operator/deploy/operator.yaml) changing the container image field with the newly built image.
```sh 
# Change dcota1 with your username, a version can be added with :vx.y
# replace x and y with your version number
$ operator-sdk build dcota1/student-operator

# Push the image, you need a docker.io account
$ docker push dcota1/student-operator

# Now replace the image field in the deploy/operator.yaml
# with your new image version
```
## Modify the CRD

In order to modify the Student CRD you have to modify the code in [pkg/apis/netgroup/v1/studentapi_types.go](pkg/apis/netgroup/v1/studentapi_types.go). This will build a new Schema for the Kubernetes API to access the newly created.
Each time you change that file you have to run the following commands:
```sh
$ operator-sdk generate k8s

$ operator-sdk generate openapis
```

## Delete the resources
```sh
# Delete all created CRs
$ kubectl delete -f deploy/crds/davidecota.yaml

# Delete all created ConfigMaps
$ kubectl delete -f utilities/my-server.yaml

# Delete the deployment
$ kubectl delete -f deploy/operator.yaml

# Delete the RBAC
$ kubectl delete -f deploy/role.yaml
$ kubectl delete -f deploy/role_binding.yaml
$ kubectl delete -f deploy/service_account.yaml

# Delete the CRD
$ kubectl delete -f deploy/crds/netgroup.com_studentapis_crd.yaml
```
