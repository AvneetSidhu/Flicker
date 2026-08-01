package session

import "encoding/json"

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	UserID  string          `json:"userId,omitempty"`
}

type JoinRoomPayload struct {
	RoomID string `json:"roomId"`
}

type WebRTCOfferPayload struct {
	SDP string `json:"sdp"`
}

type WebRTCAnswerPayload struct {
	SDP string `json:"sdp"`
}

type ICECandidatePayload struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex uint16 `json:"sdpMLineIndex"`
}

type CreateRoomPayload struct {
	RoomName string `json:"roomName"`
}

type RoomCreatedPayload struct {
	RoomID string `json:"roomId"`
}

type KeyboardInputPayload struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

type MouseInputPayload struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Action   string  `json:"action"`
	Dragging bool    `json:"dragging"`
}

type ScrollInputPayload struct {
	Direction string `json:"direction"`
}

type ControlStatePayload struct {
	Granted bool `json:"granted"`
}

type RoomViewer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RoomPresencePayload struct {
	RoomID  string       `json:"roomId"`
	Viewers []RoomViewer `json:"viewers"`
}
