package servicesync_test

import (
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"testing"
	"time"

	"q42/mc-robot/pkg/controller/servicesync"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
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
