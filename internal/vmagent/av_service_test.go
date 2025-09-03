package vmagent

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"
)

type mockConn struct {
	written bytes.Buffer
	failWrite bool
}

func (m *mockConn) Write(b []byte) (int, error) {
	if m.failWrite {
		return 0, fmt.Errorf("mock write error")
	}
	return m.written.Write(b)
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) Read(b []byte) (int, error)        { return 0, nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNewAVService(t *testing.T) {
    cfg := &Config{
        SFUAddr: "127.0.0.1:8080",
        Port:    "8000",
    }

	s := NewAVService(cfg)

	if s.sfuAddr != cfg.SFUAddr {
		t.Errorf("expected sfuAddr %s, got %s", cfg.SFUAddr, s.sfuAddr)
	}
	if s.port != cfg.Port {
		t.Errorf("expected port %s, got %s", cfg.Port, s.port)
	}
	if s.sfu != nil {
		t.Error("expected sfu to be nil")
	}
	if s.logger == nil {
		t.Error("expcted logger to be non-nil")
	}
}

func TestConnectToSFU(t *testing.T) {
	l, err := net.ListenPacket("udp","127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	s := &AVService{sfuAddr: l.LocalAddr().String()}

	if s.ConnectToSFU(); err != nil {
		t.Fatal("ConnectToSFU failed:", err)
	}

	if s.sfu == nil {
		t.Error("expected sfu connection to be non-nil")
	}
}