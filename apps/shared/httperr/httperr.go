package httperr

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/twitter/shared/reqlog"
)

type AppError struct {
	Status  int
	Code    string
	Message string
}

func (e *AppError) Error() string { return e.Message }

func New(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

// Shared sentinel errors — returned by service layer, mapped to HTTP by Handle.
var (
	ErrNotFound     = New(http.StatusNotFound, "NOT_FOUND", "not found")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "forbidden")
	ErrConflict     = New(http.StatusConflict, "CONFLICT", "conflict")
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
)

type ErrorResponse struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id"`
}

// Non-AppError values are logged and returned as 500.
func Handle(c *gin.Context, err error, log *slog.Logger) {
	var e *AppError
	if errors.As(err, &e) {
		write(c, e.Status, e.Code, e.Message, nil)
		return
	}
	log.Error("unhandled error", "err", err)
	Internal(c)
}

func BadRequest(c *gin.Context, code, message string) {
	write(c, http.StatusBadRequest, code, message, nil)
}

func Unauthorized(c *gin.Context, message string) {
	write(c, http.StatusUnauthorized, "UNAUTHORIZED", message, nil)
}

func Forbidden(c *gin.Context, message string) {
	write(c, http.StatusForbidden, "FORBIDDEN", message, nil)
}

func NotFound(c *gin.Context, message string) {
	write(c, http.StatusNotFound, "NOT_FOUND", message, nil)
}

func Conflict(c *gin.Context, code, message string) {
	write(c, http.StatusConflict, code, message, nil)
}

func Unprocessable(c *gin.Context, code, message string) {
	write(c, http.StatusUnprocessableEntity, code, message, nil)
}

func Internal(c *gin.Context) {
	write(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", nil)
}

func ServiceUnavailable(c *gin.Context, message string) {
	write(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", message, nil)
}

func Validation(c *gin.Context, err error) {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		BadRequest(c, "BAD_REQUEST", "invalid request body")
		return
	}
	fields := make(map[string]string, len(ve))
	for _, fe := range ve {
		fields[camelToSnake(fe.Field())] = fieldMessage(fe)
	}
	write(c, http.StatusBadRequest, "VALIDATION_ERROR", "validation failed", fields)
}

func AbortUnauthorized(c *gin.Context, message string) {
	Unauthorized(c, message)
	c.Abort()
}

func AbortForbidden(c *gin.Context, message string) {
	Forbidden(c, message)
	c.Abort()
}

func write(c *gin.Context, status int, code, message string, fields map[string]string) {
	c.JSON(status, ErrorResponse{
		Code:      code,
		Message:   message,
		Fields:    fields,
		RequestID: reqlog.GetRequestID(c),
	})
}

func fieldMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "minimum length is " + fe.Param()
	case "max":
		return "maximum length is " + fe.Param()
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	default:
		return "invalid value"
	}
}

func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
