package network

import (
	"fmt"
	"github.com/op/go-logging"
	"sync"
	"testing"
)

func TestProxySetupFailure(t *testing.T) {

	childPid := 1234
	// empty
	ozSockets := []ProxyConfig{}
	log := logging.MustGetLogger("testing")
	ready := sync.WaitGroup{}

	err := ProxySetup(childPid, ozSockets, log, ready)
	if err != nil {
		t.Errorf("ProxySetup failure: %s", err)
		t.Fail()
	}

	ozSockets = []ProxyConfig{
		{
			Nettype:     ProxyType("client"),
			Proto:       ProtoType("tcp"),
			Port:        1234,
			Destination: "127.0.0.1",
		},
	}

	err = ProxySetup(childPid, ozSockets, log, ready)
	if err == nil {
		t.Error("ProxySetup should have failed")
		t.Fail()
	} else {
		if fmt.Sprintf("%s", err) != "{Nettype:client Proto:tcp Port:1234 Destination:127.0.0.1}, Unable to set namespace" {
			t.Error("ProxySetup fail; expected a different error message.")
			t.Fail()
		}
	}
}
