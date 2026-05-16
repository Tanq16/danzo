package highway

type ProgressType int

const (
	ProgressTypeProgress ProgressType = iota
	ProgressTypeSubStatus
)

type Progress struct {
	JobID     string       `json:"jobId"`
	Type      ProgressType `json:"type"`
	Message   string       `json:"message,omitempty"`
	SubStatus string       `json:"subStatus,omitempty"`
	Current   int64        `json:"current,omitempty"`
	Total     int64        `json:"total,omitempty"`
	Extra     string       `json:"extra,omitempty"`
	Done      bool         `json:"done,omitempty"`
	Error     error        `json:"-"`
	ErrMsg    string       `json:"error,omitempty"`
}
