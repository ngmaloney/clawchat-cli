package gateway

// Gateway is the interface that both OpenClaw and ZeroClaw clients satisfy.
// App uses this interface so backends are interchangeable.
type Gateway interface {
	Connect() error
	Close()
	Status() Status
	ListSessions() ([]Session, error)
	GetHistory(sessionKey string, limit int) ([]Message, error)
	SendMessage(sessionKey, text, idempotencyKey string) (string, error)
}

// Compile-time assertion: *Client must satisfy Gateway.
var _ Gateway = (*Client)(nil)
