package servicesync_test

import (
	"testing"
	// https://godoc.org/sigs.k8s.io/controller-runtime/pkg/envtest#Environment
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestEnv(t *testing.T) {
	et := &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{Paths: []string{"../../../deploy/crds/mc.q42.nl_servicesyncs_crd.yaml"}},
	}
	et.Start()
}
