package apierror

import (
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

type Type string

const (
	TypeInvalidRequest Type = "invalid_request_error"
	TypeAuthentication Type = "authentication_error"
	TypePermission     Type = "permission_error"
	TypeNotFound       Type = "not_found_error"
	TypeConflict       Type = "conflict_error"
	TypeRateLimit      Type = "rate_limit_error"
	TypeServer         Type = "server_error"
)

type Error struct {
	Type  Type    `json:"type"`
	Code  string  `json:"code"`
	Param *string `json:"param"`
}

func (e *Error) Error() string {
	return e.Code
}

var (
	ErrInvalidRequest          = &Error{Type: TypeInvalidRequest, Code: "invalid_request"}
	ErrWeakPassword            = &Error{Type: TypeInvalidRequest, Code: "weak_password"}
	ErrPasswordUnchanged       = &Error{Type: TypeInvalidRequest, Code: "password_unchanged", Param: strPtr("new_password")}
	ErrInvalidPermission       = &Error{Type: TypeInvalidRequest, Code: "invalid_permission"}
	ErrMethodNotAllowed        = &Error{Type: TypeInvalidRequest, Code: "method_not_allowed"}
	ErrInvalidCredentials      = &Error{Type: TypeAuthentication, Code: "invalid_credentials"}
	ErrUnauthorized            = &Error{Type: TypeAuthentication, Code: "unauthorized"}
	ErrTwoFactorRequired       = &Error{Type: TypeAuthentication, Code: "two_factor_required"}
	ErrTwoFactorInvalidCode    = &Error{Type: TypeAuthentication, Code: "two_factor_invalid_code", Param: strPtr("code")}
	ErrTwoFactorLocked         = &Error{Type: TypeAuthentication, Code: "two_factor_locked"}
	ErrForbidden               = &Error{Type: TypePermission, Code: "forbidden"}
	ErrGrantForbidden          = &Error{Type: TypePermission, Code: "grant_forbidden"}
	ErrOwnerProtected          = &Error{Type: TypePermission, Code: "owner_protected"}
	ErrCannotModifySelf        = &Error{Type: TypePermission, Code: "cannot_modify_self"}
	ErrNotFound                = &Error{Type: TypeNotFound, Code: "not_found"}
	ErrPresetGroupLocked       = &Error{Type: TypeConflict, Code: "preset_group_locked"}
	ErrEmailTaken              = &Error{Type: TypeConflict, Code: "email_taken", Param: strPtr("email")}
	ErrNicknameTaken           = &Error{Type: TypeConflict, Code: "nickname_taken", Param: strPtr("nickname")}
	ErrAlreadyTaken            = &Error{Type: TypeConflict, Code: "already_taken"}
	ErrGroupNameTaken          = &Error{Type: TypeConflict, Code: "group_name_taken", Param: strPtr("name")}
	ErrGroupNotEmpty           = &Error{Type: TypeConflict, Code: "group_not_empty"}
	ErrLastOwner               = &Error{Type: TypeConflict, Code: "last_owner"}
	ErrTwoFactorAlreadyEnabled = &Error{Type: TypeConflict, Code: "two_factor_already_enabled"}
	ErrTwoFactorNotSetup       = &Error{Type: TypeConflict, Code: "two_factor_not_setup"}
	ErrTwoFactorNotEnabled     = &Error{Type: TypeConflict, Code: "two_factor_not_enabled"}
	ErrMailDeliveryFailed      = &Error{Type: TypeServer, Code: "mail_delivery_failed"}
	ErrServiceUnavailable      = &Error{Type: TypeServer, Code: "service_unavailable"}
	ErrInternal                = &Error{Type: TypeServer, Code: "internal_error"}
)

var statusByCode = map[string]int{
	"invalid_request":            http.StatusBadRequest,
	"weak_password":              http.StatusBadRequest,
	"password_unchanged":         http.StatusBadRequest,
	"invalid_permission":         http.StatusBadRequest,
	"method_not_allowed":         http.StatusMethodNotAllowed,
	"invalid_credentials":        http.StatusUnauthorized,
	"unauthorized":               http.StatusUnauthorized,
	"two_factor_required":        http.StatusUnauthorized,
	"two_factor_invalid_code":    http.StatusUnauthorized,
	"two_factor_locked":          http.StatusUnauthorized,
	"forbidden":                  http.StatusForbidden,
	"grant_forbidden":            http.StatusForbidden,
	"owner_protected":            http.StatusForbidden,
	"cannot_modify_self":         http.StatusForbidden,
	"not_found":                  http.StatusNotFound,
	"preset_group_locked":        http.StatusConflict,
	"email_taken":                http.StatusConflict,
	"nickname_taken":             http.StatusConflict,
	"already_taken":              http.StatusConflict,
	"group_name_taken":           http.StatusConflict,
	"group_not_empty":            http.StatusConflict,
	"last_owner":                 http.StatusConflict,
	"two_factor_already_enabled": http.StatusConflict,
	"two_factor_not_setup":       http.StatusConflict,
	"two_factor_not_enabled":     http.StatusConflict,
	"mail_delivery_failed":       http.StatusInternalServerError,
	"service_unavailable":        http.StatusServiceUnavailable,
	"internal_error":             http.StatusInternalServerError,
}

var unavailableSQLStateClasses = map[string]bool{
	"08": true,
	"53": true,
	"57": true,
	"58": true,
}

func isUnavailable(err error) bool {
	if err == nil {
		return false
	}

	var connectErr *pgconn.ConnectError
	if errors.As(err, &connectErr) {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && len(pgErr.Code) >= 2 && unavailableSQLStateClasses[pgErr.Code[:2]] {
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func resolve(err error) (int, *Error) {
	if isUnavailable(err) {
		err = ErrServiceUnavailable
	}
	return statusOf(err), Body(err)
}

type paramOverride struct {
	err   *Error
	param string
}

func (p paramOverride) Error() string {
	return p.err.Error()
}

func (p paramOverride) Unwrap() error {
	return p.err
}

func strPtr(value string) *string {
	return &value
}

func WithParam(err error, param string) error {
	switch target := err.(type) {
	case *Error:
		return paramOverride{err: target, param: param}
	case paramOverride:
		return paramOverride{err: target.err, param: param}
	default:
		return err
	}
}

func Body(err error) *Error {
	var target *Error
	if !errors.As(err, &target) {
		return internalBody()
	}
	if _, ok := statusByCode[target.Code]; !ok {
		return internalBody()
	}
	out := *target
	if target.Param != nil {
		out.Param = strPtr(*target.Param)
	}
	var override paramOverride
	if errors.As(err, &override) {
		out.Param = strPtr(override.param)
	}
	return &out
}

func internalBody() *Error {
	out := *ErrInternal
	if ErrInternal.Param != nil {
		out.Param = strPtr(*ErrInternal.Param)
	}
	return &out
}

func statusOf(err error) int {
	var target *Error
	if errors.As(err, &target) {
		if status, ok := statusByCode[target.Code]; ok {
			return status
		}
	}
	return http.StatusInternalServerError
}

func Write(ctx *gin.Context, err error) {
	status, body := resolve(err)
	ctx.JSON(status, gin.H{"error": body})
}

func Abort(ctx *gin.Context, err error) {
	status, body := resolve(err)
	ctx.AbortWithStatusJSON(status, gin.H{"error": body})
}
