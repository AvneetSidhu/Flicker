package control

type EventType string

const (
	KeyboardEvent EventType = "keyboard"
	MouseEvent    EventType = "mouse"
	ScrollEvent   EventType = "scroll"
)

type CommandEvent struct {
	Type EventType

	Key    string
	Action string

	X        float64
	Y        float64
	Dragging bool

	Direction string

	RoomID string
}
