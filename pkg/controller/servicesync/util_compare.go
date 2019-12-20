package servicesync

import (
	mcv1 "q42/mc-robot/pkg/apis/mc/v1"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func operatorStatusesEqual(a, b mcv1.ServiceSyncStatus) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b mcv1.PeerService) bool { return strings.Compare(a.ServiceName, b.ServiceName) < 0 }),
		cmpopts.SortSlices(func(a, b mcv1.PeerEndpoint) bool { return strings.Compare(a.IPAddress, b.IPAddress) < 0 }),
		cmpopts.SortSlices(func(a, b string) bool { return strings.Compare(a, b) < 0 }),
		cmpopts.IgnoreFields(mcv1.ServiceSyncStatus{}, "LastPublishTime"),
	}
	if !cmp.Equal(a, b, conditionCmpOpts...) {
		// For debugging [operatorStatusesEqual], uncomment the following:
		// if diff := cmp.Diff(a, b, conditionCmpOpts...); diff != "" {
		// 	log.Info(fmt.Sprintf("Diff mismatch (-want +got):\n%s", diff))
		// }
		return false
	}
	return true
}
