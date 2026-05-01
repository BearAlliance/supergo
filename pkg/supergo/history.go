package supergo

import (
	"net/http"
	"time"
)

// HistoryEntry records one complete request/response cycle performed by an Agent.
type HistoryEntry struct {
	Method     string
	Path       string
	ReqHeader  http.Header
	Response   *Response
	Assertions []string // human-readable description of each assertion that ran
	ExecutedAt time.Time
}
