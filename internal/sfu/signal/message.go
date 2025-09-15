package signal

import "encoding/json"

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	UserID  string          `json:"userId,omitempty"`
}

type JoinRoomPayload struct {
	RoomID uint32 `json:"roomId"`
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
