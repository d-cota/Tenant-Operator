package controller

import (
	"github.com/example-inc/memcached-operator/pkg/controller/studentapi"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, studentapi.Add)
}
