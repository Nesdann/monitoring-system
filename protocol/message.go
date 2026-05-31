package protocol

type Message struct {
	Type      string `json:"type"`
	Hostname  string `json:"hostname,omitempty"`
	Timestamp int64  `json:"timestamp"`
	CPU       float64 `json:"cpu,omitempty"`
	RAM       float64 `json:"ram,omitempty"`
}

type Event struct {
    Type      string         `json:"type"`
    Timestamp int64          `json:"timestamp"`
    Hostname  string         `json:"hostname"`
    Data      map[string]any `json:"data"`
}