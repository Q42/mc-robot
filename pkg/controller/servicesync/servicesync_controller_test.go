package servicesync_test

import (
	"fmt"
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"testing"
	"time"

	"q42/mc-robot/pkg/controller/servicesync"

	// https://godoc.org/sigs.k8s.io/controller-runtime/pkg/envtest#Environment
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestPrune(t *testing.T) {
	clusters := make(map[string]*mcv1.Cluster, 0)
	clusters["foo"] = &mcv1.Cluster{Name: "foo", LastUpdate: v1.Time{Time: time.Now()}}
	clusters["bar"] = &mcv1.Cluster{Name: "bar", LastUpdate: v1.Time{Time: time.Now().Add(-1 * time.Hour)}}
	pruned := servicesync.PruneExpired(&clusters, "1800s")
	if len(clusters) != 1 {
		t.Error("pruneExpired did not remove the cluster from the map that was provided")
	}
	if len(pruned) != 1 || pruned[0] != "bar" {
		t.Error("pruneExpired did not correctly report changes")
	}
}

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
