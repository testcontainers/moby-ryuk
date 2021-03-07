package main

import (
	"net"
	"strings"
	"testing"
	"time"
)

var addr = &net.TCPAddr{
	IP:   net.IPv4zero,
	Port: 5555,
	Zone: "",
}

func init() {
	initialConnectTimeout = 5 * time.Second
	reconnectionTimeout = 1 * time.Second
}

func TestReconnectionTimeout(t *testing.T) {
	acc := make(chan net.Addr)
	lost := make(chan net.Addr)

	done := make(chan struct{})

	go func() {
		waitForPruneCondition(acc, lost)
		done <- struct{}{}
	}()

	acc <- addr
	lost <- addr

	select {
	case <-done:
		return
	case <-time.After(2 * time.Second):
		t.Fail()
	}
}

func TestInitialTimeout(t *testing.T) {
	acc := make(chan net.Addr)
	lost := make(chan net.Addr)

	done := make(chan string)

	go func() {
		defer func() {
			err := recover().(string)
			done <- err
		}()
		waitForPruneCondition(acc, lost)
	}()

	select {
	case p := <-done:
		if !strings.Contains(p, "first connection") {
			t.Fail()
		}
	case <-time.After(7 * time.Second):
		t.Fail()
	}
}
