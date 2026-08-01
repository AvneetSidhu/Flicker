package room

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
	"github.com/pion/rtp"
)

func newTestSession(id string) *session.Session {
	return session.NewSession(id, id, nil, nil, slog.Default(), nil, nil)
}

func TestRoomBroadcastsPresenceOnJoinAndLeave(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	first := newTestSession("first")
	second := newTestSession("second")

	room.AddMember(first)
	assertPresence(t, <-first.SendChan, "room", []string{"first"})

	room.AddMember(second)
	assertPresence(t, <-first.SendChan, "room", []string{"first", "second"})
	assertPresence(t, <-second.SendChan, "room", []string{"first", "second"})

	room.RemoveMember(second)
	assertPresence(t, <-first.SendChan, "room", []string{"first"})
}

func assertPresence(t *testing.T, encoded []byte, roomID string, viewerIDs []string) {
	t.Helper()
	var message session.WSMessage
	if err := json.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("invalid presence message: %v", err)
	}
	if message.Type != "room-presence" {
		t.Fatalf("message type = %q, want room-presence", message.Type)
	}
	var payload session.RoomPresencePayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("invalid presence payload: %v", err)
	}
	if payload.RoomID != roomID || len(payload.Viewers) != len(viewerIDs) {
		t.Fatalf("unexpected presence payload: %#v", payload)
	}
	for index, id := range viewerIDs {
		if payload.Viewers[index].ID != id {
			t.Fatalf("viewer %d = %q, want %q", index, payload.Viewers[index].ID, id)
		}
	}
}

func TestRoomTracksVMProvisioningState(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	if room.VMState() != VMStateProvisioning || room.IsVMReady() {
		t.Fatalf("unexpected initial VM state: %q", room.VMState())
	}

	room.SetVMFailed(errors.New("clone failed"))
	if room.VMState() != VMStateFailed || room.VMFailure() != "clone failed" || room.IsVMReady() {
		t.Fatalf("unexpected failed VM state: state=%q failure=%q", room.VMState(), room.VMFailure())
	}

	room.SetVMReady()
	if room.VMState() != VMStateReady || !room.IsVMReady() {
		t.Fatalf("unexpected ready VM state: %q", room.VMState())
	}
}

func TestRoomMembershipRejectsDuplicatesAndTracksEmptyState(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	member := newTestSession("member")

	if !room.AddMember(member) {
		t.Fatal("expected first member to be added")
	}
	if room.AddMember(member) {
		t.Fatal("expected duplicate member to be rejected")
	}
	if room.MemberCount() != 1 || room.EmptyDuration() != 0 {
		t.Fatalf("unexpected room state: count=%d empty=%s", room.MemberCount(), room.EmptyDuration())
	}
	if !room.RemoveMember(member) {
		t.Fatal("expected member removal to succeed")
	}
	if room.RemoveMember(member) {
		t.Fatal("expected absent member removal to fail")
	}
	if room.MemberCount() != 0 || room.emptyAt.IsZero() {
		t.Fatalf("expected empty room state, got count=%d emptyAt=%s", room.MemberCount(), room.emptyAt)
	}
}

func TestRoomReleasesControlWhenControllerLeaves(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	member := newTestSession("member")
	room.AddMember(member)
	if !room.RequestControl(member.UserID) {
		t.Fatal("expected control request to succeed")
	}
	room.RemoveMember(member)
	if room.CurrentController != "" {
		t.Fatalf("controller was not released: %q", room.CurrentController)
	}
}

func TestRoomMembershipHandlesConcurrentJoins(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	const members = 100
	var group sync.WaitGroup
	for index := 0; index < members; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			room.AddMember(newTestSession(fmt.Sprintf("member-%d", index)))
		}(index)
	}
	group.Wait()
	if room.MemberCount() != members {
		t.Fatalf("expected %d members, got %d", members, room.MemberCount())
	}
}

func TestRoomBroadcastDeliversToActiveMembersAndReleasesPacket(t *testing.T) {
	room := NewRoom(slog.Default(), "room", "Room", "owner", "owner-id", nil)
	member := newTestSession("member")
	member.SetActiveStream(true)
	room.AddMember(member)

	released := make(chan []byte, 1)
	packet := ingest.NewPooledPacket(&rtp.Packet{Payload: []byte("payload")}, []byte("buffer"), func(buffer []byte) {
		released <- buffer
	})

	room.Broadcast(packet, "audio")
	select {
	case received := <-member.AudioBuf:
		if string(received.Packet.Payload) != "payload" {
			t.Fatalf("unexpected payload: %q", received.Packet.Payload)
		}
		received.Release()
	case <-time.After(time.Second):
		t.Fatal("active member did not receive audio packet")
	}

	select {
	case buffer := <-released:
		if string(buffer) != "buffer" {
			t.Fatalf("unexpected released buffer: %q", buffer)
		}
	case <-time.After(time.Second):
		t.Fatal("packet buffer was not released")
	}
}
