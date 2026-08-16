package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
)

const (
	actionUseReadTools             = "use_read_tools"
	actionCallLoginTool            = "call_get_login_qrcode"
	actionScanQRCode               = "scan_qr_code"
	actionConfirmOnPhone           = "confirm_on_phone"
	actionWaitForLoginVerification = "wait_for_login_verification"
	actionRestartLogin             = "restart_login"
	actionInspectHealth            = "inspect_health"
	actionRetry                    = "retry"
	actionCorrectInput             = "correct_input"

	loginActionPath  = "/login"
	healthActionPath = "/health"
)

type requestIDContextKey struct{}

// PublicError is the stable, secret-free error contract shared by HTTP and MCP.
type PublicError struct {
	Source     string `json:"source"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"next_action,omitempty"`
	ActionPath string `json:"action_path,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

type toolOutput[T any] struct {
	OK    bool         `json:"ok"`
	Data  *T           `json:"data,omitempty"`
	Error *PublicError `json:"error,omitempty"`
}

type classifiedPublicError struct {
	StatusCode int
	Error      PublicError
}

func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func successfulToolOutput[T any](data T) toolOutput[T] {
	return toolOutput[T]{
		OK:   true,
		Data: &data,
	}
}

func failedToolOutput[T any](publicError PublicError) toolOutput[T] {
	return toolOutput[T]{
		OK:    false,
		Error: &publicError,
	}
}

func invalidArgumentPublicError(ctx context.Context, message string) PublicError {
	return PublicError{
		Source:     "input",
		Code:       "INVALID_ARGUMENT",
		Message:    message,
		Retryable:  false,
		NextAction: actionCorrectInput,
		RequestID:  requestIDFromContext(ctx),
	}
}

func internalPublicError(ctx context.Context, message string) PublicError {
	return PublicError{
		Source:    "server",
		Code:      "INTERNAL_ERROR",
		Message:   message,
		Retryable: false,
		RequestID: requestIDFromContext(ctx),
	}
}

func classifyPublicError(
	ctx context.Context,
	fallbackCode string,
	fallbackMessage string,
	err error,
) classifiedPublicError {
	classified := classifiedPublicError{
		StatusCode: http.StatusInternalServerError,
		Error: PublicError{
			Source:    "server",
			Code:      fallbackCode,
			Message:   fallbackMessage,
			Details:   safeErrorText(err),
			Retryable: false,
			RequestID: requestIDFromContext(ctx),
		},
	}

	var invalidArgument *xiaohongshu.InvalidArgumentError
	var operationTimeout *operationTimeoutError
	var queueTimeout *operationQueueTimeoutError
	var gateUnavailable *accessGateUnavailableError
	var browserUnavailable *browserRuntimeUnavailableError

	switch {
	case errors.As(err, &invalidArgument):
		classified.StatusCode = http.StatusBadRequest
		classified.Error.Source = "input"
		classified.Error.Code = "INVALID_ARGUMENT"
		classified.Error.Message = invalidArgument.Error()
		classified.Error.Details = ""
		classified.Error.NextAction = actionCorrectInput
	case errors.As(err, &operationTimeout):
		classified.StatusCode = http.StatusGatewayTimeout
		classified.Error.Code = "OPERATION_TIMEOUT"
		classified.Error.Message = "Browser operation timed out"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionInspectHealth
		classified.Error.ActionPath = healthActionPath
	case errors.As(err, &queueTimeout):
		classified.StatusCode = http.StatusServiceUnavailable
		classified.Error.Code = "SERVICE_BUSY"
		classified.Error.Message = "Browser operation could not start"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionInspectHealth
		classified.Error.ActionPath = healthActionPath
	case errors.As(err, &gateUnavailable):
		classified.StatusCode = http.StatusServiceUnavailable
		classified.Error.Code = "SERVICE_DEGRADED"
		classified.Error.Message = "A previous browser operation is still stopping"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionInspectHealth
		classified.Error.ActionPath = healthActionPath
	case errors.As(err, &browserUnavailable):
		classified.StatusCode = http.StatusServiceUnavailable
		classified.Error.Code = "BROWSER_UNAVAILABLE"
		classified.Error.Message = "Browser runtime is unavailable"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionInspectHealth
		classified.Error.ActionPath = healthActionPath
	case errors.Is(err, context.DeadlineExceeded):
		classified.StatusCode = http.StatusGatewayTimeout
		classified.Error.Code = "OPERATION_TIMEOUT"
		classified.Error.Message = "Browser operation timed out"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionInspectHealth
		classified.Error.ActionPath = healthActionPath
	case errors.Is(err, context.Canceled):
		classified.StatusCode = http.StatusRequestTimeout
		classified.Error.Code = "REQUEST_CANCELED"
		classified.Error.Message = "Request was canceled"
		classified.Error.Retryable = true
		classified.Error.NextAction = actionRetry
	}

	return classified
}
