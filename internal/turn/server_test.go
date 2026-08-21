package turn

import (
	"net"
	"strconv"
	"testing"
)

func TestPort(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("TURN_PORT", "")
		port, err := Port()
		if err != nil || port != defaultPort {
			t.Fatalf("Port() = %d, %v", port, err)
		}
	})
	t.Run("configured", func(t *testing.T) {
		t.Setenv("TURN_PORT", "5349")
		port, err := Port()
		if err != nil || port != 5349 {
			t.Fatalf("Port() = %d, %v", port, err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		t.Setenv("TURN_PORT", "invalid")
		if _, err := Port(); err == nil {
			t.Fatal("Port() succeeded with an invalid value")
		}
	})
	t.Run("zero", func(t *testing.T) {
		t.Setenv("TURN_PORT", "0")
		if _, err := Port(); err == nil {
			t.Fatal("Port() succeeded with port zero")
		}
	})
}

func TestStartReturnsGeneratedSecret(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.LocalAddr().(*net.UDPAddr).Port
	listener.Close()

	t.Setenv("TURN_PORT", strconv.Itoa(port))
	t.Setenv("TURN_SECRET", "")
	t.Setenv("TURN_PUBLIC_IP", "127.0.0.1")

	server, secret, err := Start("discard")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if secret == "" {
		t.Fatal("Start() returned an empty generated secret")
	}
}
