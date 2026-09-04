package errors

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrNoHealthyBackends    = errors.New("no healthy backends available")
	ErrBackendUnhealthy     = errors.New("backend is unhealthy")
	ErrCircuitOpen          = errors.New("circuit breaker is open")
	ErrRequestTimeout       = errors.New("request timeout")
	ErrConnectionRefused    = errors.New("connection refused")
	ErrInvalidConfiguration = errors.New("invalid configuration")
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrInvalidRequest       = errors.New("invalid request")
	ErrResponseTooLarge     = errors.New("response too large")
	ErrRequestTooLarge      = errors.New("request too large")
	ErrIPBlocked            = errors.New("IP address blocked")
)

type ErrorType string

const (
	ErrorTypeBackend       ErrorType = "backend"
	ErrorTypeNetwork       ErrorType = "network"
	ErrorTypeConfiguration ErrorType = "configuration"
	ErrorTypeSecurity      ErrorType = "security"
	ErrorTypeRateLimit     ErrorType = "rate_limit"
	ErrorTypeValidation    ErrorType = "validation"
)

type ProxyError struct {
	Err        error
	Type       ErrorType
	StatusCode int
	Message    string
	Backend    string
}

func (pe *ProxyError) Error() string {
	if pe.Message != "" {
		return pe.Message
	}
	return pe.Err.Error()
}

func (pe *ProxyError) Unwrap() error {
	return pe.Err
}

func NewProxyError(err error, errorType ErrorType, statusCode int, message string) *ProxyError {
	return &ProxyError{
		Err:        err,
		Type:       errorType,
		StatusCode: statusCode,
		Message:    message,
	}
}

func NewBackendError(err error, backend string) *ProxyError {
	return &ProxyError{
		Err:        err,
		Type:       ErrorTypeBackend,
		StatusCode: http.StatusBadGateway,
		Message:    fmt.Sprintf("backend error: %s", backend),
		Backend:    backend,
	}
}

func NewNetworkError(err error) *ProxyError {
	return &ProxyError{
		Err:        err,
		Type:       ErrorTypeNetwork,
		StatusCode: http.StatusBadGateway,
		Message:    "network error",
	}
}

func NewConfigurationError(err error) *ProxyError {
	return &ProxyError{
		Err:        err,
		Type:       ErrorTypeConfiguration,
		StatusCode: http.StatusInternalServerError,
		Message:    "configuration error",
	}
}

func NewSecurityError(err error, message string) *ProxyError {
	return &ProxyError{
		Err:        err,
		Type:       ErrorTypeSecurity,
		StatusCode: http.StatusForbidden,
		Message:    message,
	}
}

func NewRateLimitError() *ProxyError {
	return &ProxyError{
		Err:        ErrRateLimitExceeded,
		Type:       ErrorTypeRateLimit,
		StatusCode: http.StatusTooManyRequests,
		Message:    "rate limit exceeded",
	}
}

func NewValidationError(message string) *ProxyError {
	return &ProxyError{
		Err:        ErrInvalidRequest,
		Type:       ErrorTypeValidation,
		StatusCode: http.StatusBadRequest,
		Message:    message,
	}
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var proxyErr *ProxyError
	if errors.As(err, &proxyErr) {
		switch proxyErr.Type {
		case ErrorTypeNetwork, ErrorTypeBackend:
			return true
		case ErrorTypeConfiguration, ErrorTypeSecurity, ErrorTypeValidation:
			return false
		}
	}

	return errors.Is(err, ErrRequestTimeout) ||
		errors.Is(err, ErrConnectionRefused) ||
		errors.Is(err, ErrBackendUnhealthy)
}
