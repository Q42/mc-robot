package servicesync

import (
	"math"
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var conditionCmpOpts = []cmp.Option{
	cmpopts.EquateEmpty(),
	cmpopts.SortMaps(func(a, b string) bool { return strings.Compare(a, b) < 0 }),
	cmpopts.SortSlices(func(a, b mcv1.Cluster) bool { return strings.Compare(a.Name, b.Name) < 0 }),
	cmpopts.SortSlices(func(a, b mcv1.PeerService) bool { return strings.Compare(a.ServiceName, b.ServiceName) < 0 }),
	cmpopts.SortSlices(func(a, b mcv1.PeerEndpoint) bool { return strings.Compare(a.IPAddress, b.IPAddress) < 0 }),
	cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 }),
	// Don't write change more frequent than (1/30)Hz, if only the publish time changed
	cmp.Comparer(func(x, y metav1.Time) bool {
		delta := x.Time.Sub(y.Time)
		return math.Abs(delta.Seconds()) < (30 * time.Second).Seconds()
	}),
}

func operatorStatusesEqual(a, b mcv1.ServiceSyncStatus) bool {
	if !cmp.Equal(a, b, conditionCmpOpts...) {
		// For debugging [operatorStatusesEqual], uncomment the following:
		// if diff := cmp.Diff(a, b, conditionCmpOpts...); diff != "" {
		// 	log.Info(fmt.Sprintf("Diff mismatch (-want +got):\n%s", diff))
		// }
		return false
	}
	return true
}

func operatorPeerServicesEqual(a, b map[string]*mcv1.PeerService) (bool, string) {
	if !cmp.Equal(a, b, conditionCmpOpts...) {
		// For debugging [operatorStatusesEqual], uncomment the following:
		if diff := cmp.Diff(a, b, conditionCmpOpts...); diff != "" {
			// log.Info(fmt.Sprintf("Diff mismatch (-want +got):\n%s", diff))
			return false, diff
		}
		return false, ""
	}
	return true, ""
}

// mapContains returns whether a includes all keys & values set in b. So a may contain more than b, but should at least include b.
func mapContains(a, b map[string]string) bool {
	for key, bValue := range b {
		if aValue, aHasKey := a[key]; aHasKey && aValue == bValue {
			continue
		}
		return false
	}
	return true
}

func mapMerge(a, b map[string]string) map[string]string {
	if a == nil {
		a = make(map[string]string, 0)
	}
	for key, bValue := range b {
		a[key] = bValue
	}
	return a
}
