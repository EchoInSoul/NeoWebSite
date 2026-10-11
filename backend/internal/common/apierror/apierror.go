package apierror

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
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
	ErrInvalidRequest     = &Error{Type: TypeInvalidRequest, Code: "invalid_request"}
	ErrWeakPassword       = &Error{Type: TypeInvalidRequest, Code: "weak_password"}
	ErrPasswordUnchanged  = &Error{Type: TypeInvalidRequest, Code: "password_unchanged", Param: strPtr("new_password")}
	ErrInvalidPermission  = &Error{Type: TypeInvalidRequest, Code: "invalid_permission"}
	ErrInvalidCredentials = &Error{Type: TypeAuthentication, Code: "invalid_credentials"}
	ErrUnauthorized       = &Error{Type: TypeAuthentication, Code: "unauthorized"}
	ErrForbidden          = &Error{Type: TypePermission, Code: "forbidden"}
	ErrGrantForbidden     = &Error{Type: TypePermission, Code: "grant_forbidden"}
	ErrOwnerProtected     = &Error{Type: TypePermission, Code: "owner_protected"}
	ErrCannotModifySelf   = &Error{Type: TypePermission, Code: "cannot_modify_self"}
	ErrNotFound           = &Error{Type: TypeNotFound, Code: "not_found"}
	ErrPresetGroupLocked  = &Error{Type: TypeConflict, Code: "preset_group_locked"}
	ErrEmailTaken         = &Error{Type: TypeConflict, Code: "email_taken", Param: strPtr("email")}
	ErrNicknameTaken      = &Error{Type: TypeConflict, Code: "nickname_taken", Param: strPtr("nickname")}
	ErrAlreadyTaken       = &Error{Type: TypeConflict, Code: "already_taken"}
	ErrGroupNameTaken     = &Error{Type: TypeConflict, Code: "group_name_taken", Param: strPtr("name")}
	ErrGroupNotEmpty      = &Error{Type: TypeConflict, Code: "group_not_empty"}
	ErrLastOwner          = &Error{Type: TypeConflict, Code: "last_owner"}
	ErrInternal           = &Error{Type: TypeServer, Code: "internal_error"}
)

var statusByCode = map[string]int{
	"invalid_request":     http.StatusBadRequest,
	"weak_password":       http.StatusBadRequest,
	"password_unchanged":  http.StatusBadRequest,
	"invalid_permission":  http.StatusBadRequest,
	"invalid_credentials": http.StatusUnauthorized,
	"unauthorized":        http.StatusUnauthorized,
	"forbidden":           http.StatusForbidden,
	"grant_forbidden":     http.StatusForbidden,
	"owner_protected":     http.StatusForbidden,
	"cannot_modify_self":  http.StatusForbidden,
	"not_found":           http.StatusNotFound,
	"preset_group_locked": http.StatusConflict,
	"email_taken":         http.StatusConflict,
	"nickname_taken":      http.StatusConflict,
	"already_taken":       http.StatusConflict,
	"group_name_taken":    http.StatusConflict,
	"group_not_empty":     http.StatusConflict,
	"last_owner":          http.StatusConflict,
	"internal_error":      http.StatusInternalServerError,
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
	target, ok := err.(*Error)
	if !ok {
		return err
	}
	return paramOverride{err: target, param: param}
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
	var override paramOverride
	if errors.As(err, &override) {
		out.Param = strPtr(override.param)
	}
	return &out
}

func internalBody() *Error {
	out := *ErrInternal
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
	ctx.JSON(statusOf(err), gin.H{"error": Body(err)})
}

func Abort(ctx *gin.Context, err error) {
	ctx.AbortWithStatusJSON(statusOf(err), gin.H{"error": Body(err)})
}
