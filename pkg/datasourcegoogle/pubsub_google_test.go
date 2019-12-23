package datasourcegoogle

import (
	"testing"
)

func TestTopicParse(t *testing.T) {
	match := urlSyntax.FindStringSubmatch("gcppubsub://projects/myproject/subscriptions/testsub")
	if match[1] != "myproject" {
		t.Error("match[0] should have been 'myproject' but was", match[1])
	}
	if match[2] != "subscriptions" {
		t.Error("match[2] should have been 'subscriptions' but was", match[2])
	}
	if match[3] != "testsub" {
		t.Error("match[3] should have been 'testsub' but was", match[3])
	}
}

func orPanic(err error) {
	if err != nil {
		panic(err)
	}
}
