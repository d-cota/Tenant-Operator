package controller

import (
	"github.com/netgroup/tenant-operator/pkg/controller/host"
	"github.com/netgroup/tenant-operator/pkg/controller/tenant"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, tenant.Add, host.Add)
}
