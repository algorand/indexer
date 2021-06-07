package types

// ConsistencyError is returned when the database returns inconsistent (stale) results.
type ConsistencyError struct {
	msg string
}

// MakeConsistencyError creates a new consistency error object.
func MakeConsistencyError(msg string) ConsistencyError {
	return ConsistencyError{msg}
}

func (e ConsistencyError) Error() string {
	return "consistency error: " + e.msg
}
