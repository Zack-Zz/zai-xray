package providers

import "fmt"

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.StatusCode == 0 {
		return "provider api error"
	}
	return fmt.Sprintf("provider api error (status=%d)", e.StatusCode)
}
