package protocol

type Event struct {
	Type      string         `json:"type"`
	Timestamp int64          `json:"timestamp"`
	Hostname  string         `json:"hostname"`
	Data      map[string]any `json:"data"`
}
