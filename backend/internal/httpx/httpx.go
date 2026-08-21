// Package httpx holds the shared HTTP response conventions so every endpoint
// returns the same shapes. The Flutter client parses one error format, not one
// per handler.
package httpx

import "github.com/gin-gonic/gin"

// Error codes. Stable strings — the client branches on these, not on prose.
const (
	CodeValidation     = "validation_error"
	CodeUnauthorized   = "unauthorized"
	CodeForbidden      = "forbidden"
	CodeNotFound       = "not_found"
	CodeMethodNotAllow = "method_not_allowed"
	CodeConflict       = "conflict"
	CodeInternal       = "internal_error"
	CodeUnavailable    = "service_unavailable"
)

// ErrorBody is the envelope for every non-2xx response:
//
//	{"error": {"code": "not_found", "message": "submission not found"}}
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a machine-readable code and a human-readable message.
// Fields holds per-field validation messages when code is validation_error.
type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Fail writes an error response and stops the handler chain.
func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorBody{ErrorDetail{Code: code, Message: message}})
}

// FailFields writes a validation error carrying per-field messages.
func FailFields(c *gin.Context, status int, code, message string, fields map[string]string) {
	c.AbortWithStatusJSON(status, ErrorBody{ErrorDetail{Code: code, Message: message, Fields: fields}})
}
