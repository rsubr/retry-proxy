package jobs

import "time"

type State string

const (
	StateQueued     State = "queued"
	StateProcessing State = "processing"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateExpired    State = "expired"
)

type Job struct {
	ID                  int64
	RouteName           string
	Method              string
	RequestPath         string
	QueryString         string
	HeadersJSON         string
	Body                []byte
	State               State
	RetryCount          int
	NextRetryAt         time.Time
	DeadlineAt          time.Time
	ResponseCode        *int
	ResponseHeadersJSON *string
	ResponseBody        []byte
	LastError           *string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CompletedAt         *time.Time
}
