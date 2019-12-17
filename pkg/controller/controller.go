package controller

import (
	"q42/mc-robot/pkg/datasource"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager, datasource.ExternalSource) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, ds datasource.ExternalSource) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m, ds); err != nil {
			return err
		}
	}
	return nil
}
