package ptr

func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Use when converting proto string fields (always a string, never nil) to nullable JSON fields.
func NonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
