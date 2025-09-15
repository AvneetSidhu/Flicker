package av_service

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"
)

type mockConn struct {
	written   bytes.Buffer
	failWrite bool
}

func (m *mockConn) Write(b []byte) (int, error) {
	if m.failWrite {
		return 0, fmt.Errorf("mock write error")
	}
	return m.written.Write(b)
}

func (m *mockConn) Close() error                           { return nil }
func (m *mockConn) Read(b []byte) (int, error)             { return 0, nil }
func (m *mockConn) LocalAddr() net.Addr                    { return nil }
func (m *mockConn) RemoteAddr() net.Addr                   { return nil }
func (m *mockConn) SetDeadline(t time.Time) error          { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error     { return nil }
func (m *mockConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, nil }
func (m *mockConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, nil }

func TestNewAVService(t *testing.T) {

	sfuConn := &mockConn{}
	listenConn := &mockConn{}

	s := NewAVService(sfuConn, listenConn, slog.Default().With("service", "av"))

	if s.sfu == nil {
		t.Error("expected sfu connection to be set")
	}

	if s.listen == nil {
		t.Error("expected listen connection to be set")
	}

	if s.logger == nil {
		t.Error("expcted logger to be non-nil")
	}
}

func TestForward(t *testing.T) {
	conn := &mockConn{}

	svc := &AVService{
		sfu:    nil,
		logger: slog.Default().With("service", "av"),
	}

	packet := []byte("test rtp packet")

	err := svc.forward(packet)
	if err == nil {
		t.Fatal("expected error when sfu is nil")
	}

	svc.sfu = conn

	err = svc.forward(packet)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !bytes.Equal(conn.written.Bytes(), packet) {
		t.Errorf("expected %v, got %v", packet, conn.written.Bytes())
	}

	conn.failWrite = true
	err = svc.forward(packet)
	if err == nil {
		t.Error("expected error when failWrite is true")
	}
}
