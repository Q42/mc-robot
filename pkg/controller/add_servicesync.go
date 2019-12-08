package controller

import (
	"q42/mc-robot/pkg/controller/servicesync"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, servicesync.Add)
}
