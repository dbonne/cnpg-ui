package auth

// SessionValidatorAdapter wraps Service to satisfy the middleware.SessionValidator
// interface (which returns (string, error) not (*Session, error)).
// This avoids an import cycle: middleware → auth → middleware.
type SessionValidatorAdapter struct {
	svc Service
}

// NewSessionValidatorAdapter wraps a Service to satisfy middleware.SessionValidator.
func NewSessionValidatorAdapter(svc Service) *SessionValidatorAdapter {
	return &SessionValidatorAdapter{svc: svc}
}

// ValidateSession implements middleware.SessionValidator.
// It returns the username string (not the full Session) on success.
func (a *SessionValidatorAdapter) ValidateSession(sessionID string) (string, error) {
	sess, err := a.svc.ValidateSession(sessionID)
	if err != nil {
		return "", err
	}
	return sess.Username, nil
}
