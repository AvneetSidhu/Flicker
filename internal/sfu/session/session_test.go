package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
)

type fakeRoom struct {
	added   []*Session
	removed []*Session
}

func (r *fakeRoom) AddMember(session *Session) bool {
	r.added = append(r.added, session)
	return true
}

func (r *fakeRoom) RemoveMember(session *Session) bool {
	r.removed = append(r.removed, session)
	return true
}

func (r *fakeRoom) RequestControl(string) bool                       { return true }
func (r *fakeRoom) ReleaseControl(string) bool                       { return true }
func (r *fakeRoom) ForwardInput(string, *control.CommandEvent) error { return nil }

type fakeRoomManager struct {
	rooms map[string]Room
}

func (m *fakeRoomManager) GetRoom(roomID string) (Room, bool) {
	room, ok := m.rooms[roomID]
	return room, ok
}

func (m *fakeRoomManager) CreateRoom(context.Context, string, string, string) (string, error) {
	return "created-room", nil
}

func newTestSession(manager roomManager) *Session {
	return NewSession("viewer", "user", nil, nil, slog.Default(), manager, nil)
}

func TestParseWSMessageAndPayload(t *testing.T) {
	message, err := parseWSMessage([]byte(`{"type":"join-room","payload":{"roomId":"room"}}`))
	if err != nil {
		t.Fatalf("parseWSMessage returned error: %v", err)
	}
	payload, err := parseWSMessagePayload[JoinRoomPayload](message.Payload)
	if err != nil || payload.RoomID != "room" {
		t.Fatalf("unexpected payload: %#v, error=%v", payload, err)
	}
	if _, err := parseWSMessage([]byte("not json")); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestJoinRoomOnlyRecordsExistingRoom(t *testing.T) {
	room := &fakeRoom{}
	manager := &fakeRoomManager{rooms: map[string]Room{"room": room}}
	session := newTestSession(manager)

	if err := session.joinRoom("missing"); err == nil {
		t.Fatal("expected missing-room error")
	}
	if session.roomID != "" {
		t.Fatalf("missing room should not be recorded, got %q", session.roomID)
	}
	if err := session.joinRoom("room"); err != nil {
		t.Fatalf("joinRoom returned error: %v", err)
	}
	if session.roomID != "room" || len(room.added) != 1 || room.added[0] != session {
		t.Fatalf("unexpected join state: roomID=%q added=%d", session.roomID, len(room.added))
	}
}

func TestSendRoomCreatedProducesProtocolMessage(t *testing.T) {
	session := newTestSession(&fakeRoomManager{})
	session.sendRoomCreated("room-123")

	encoded := <-session.SendChan
	var message WSMessage
	if err := json.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("invalid WS message: %v", err)
	}
	if message.Type != "room-created" {
		t.Fatalf("unexpected message type: %q", message.Type)
	}
	payload, err := parseWSMessagePayload[RoomCreatedPayload](message.Payload)
	if err != nil || payload.RoomID != "room-123" {
		t.Fatalf("unexpected room-created payload: %#v, error=%v", payload, err)
	}
}

func TestCleanupClosesResourcesWhenRoomWasAlreadyDeleted(t *testing.T) {
	session := newTestSession(&fakeRoomManager{rooms: map[string]Room{}})
	session.roomID = "deleted-room"
	session.SetActiveStream(true)

	session.CleanUpSession()

	if _, ok := <-session.AudioBuf; ok {
		t.Fatal("audio buffer was not closed")
	}
	if _, ok := <-session.VideoBuf; ok {
		t.Fatal("video buffer was not closed")
	}
	if _, ok := <-session.SendChan; ok {
		t.Fatal("send channel was not closed")
	}
}
