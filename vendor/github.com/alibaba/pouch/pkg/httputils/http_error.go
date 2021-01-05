package httputils

// HTTPError represents an HTTP error which contains potential status code.
// For API layer, daemon side should return error message and using correct status code
// to construct response when an error happens in handling requests.
type HTTPError struct {
	message    string
	statusCode int
}

// NewHTTPError returns a brand new HTTPError with input message and status code
func NewHTTPError(err error, code int) HTTPError {
	return HTTPError{
		message:    err.Error(),
		statusCode: code,
	}
}

// Error returns Message field of HTTPError
func (err HTTPError) Error() string {
	return err.message
}

// Code returns status code field of HTTPError
func (err HTTPError) Code() int {
	return err.statusCode
}
