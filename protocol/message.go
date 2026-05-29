package protocol

type Message struct {
	Type      string `json:"type"`
	Hostname  string `json:"hostname,omitempty"`
	Timestamp int64  `json:"timestamp"`
}