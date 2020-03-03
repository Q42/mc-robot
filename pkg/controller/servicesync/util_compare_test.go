package servicesync

import (
	"testing"
)

func TestMapMergeNil(t *testing.T) {
	mapMerge(nil, map[string]string{"test": "dummy"})
}

func TestMapMergeNilSecond(t *testing.T) {
	mapMerge(map[string]string{"test": "dummy"}, nil)
}
