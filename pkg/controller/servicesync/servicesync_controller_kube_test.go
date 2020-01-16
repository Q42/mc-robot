// +build testkube

package servicesync_test

import (
	"fmt"
	"testing"

	// https://godoc.org/sigs.k8s.io/controller-runtime/pkg/envtest#Environment

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestEnv(t *testing.T) {
	options := envtest.CRDInstallOptions{Paths: []string{"../../../deploy/crds/mc.q42.nl_servicesyncs_crd.yaml"}}
	et := &envtest.Environment{CRDInstallOptions: options}
	cnf, err := et.Start()
	fmt.Printf("%#v", cnf)
	orPanic(err)
}

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}
