package api

// ErrorResponse is the shared HTTP error payload.
type ErrorResponse struct {
	Error string `json:"error"`
}
