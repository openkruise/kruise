package jsonstream

import (
	"time"
)

// JSONError wraps a concrete Code and Message as error.
type JSONError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error implement the error interface.
func (e *JSONError) Error() string {
	return e.Message
}

// ProgressDetail represents the status.
type ProgressDetail struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

// JSONMessage defines a message struct for jsonstream.
// It describes id, status, progress detail, started and updated.
type JSONMessage struct {
	ID           string          `json:"id,omitempty"`
	Status       string          `json:"status,omitempty"`
	Detail       *ProgressDetail `json:"progressDetail,omitempty"`
	Error        *JSONError      `json:"errorDetail,omitempty"`
	ErrorMessage string          `json:"error,omitempty"`

	StartedAt time.Time `json:"started_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}
