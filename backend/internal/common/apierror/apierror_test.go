package apierror

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRegistryMatchesSpec(t *testing.T) {
	cases := []struct {
		name      string
		sentinel  *Error
		wantType  Type
		wantCode  string
		wantCode2 int
		wantParam *string
	}{
		{"invalid_request", ErrInvalidRequest, TypeInvalidRequest, "invalid_request", http.StatusBadRequest, nil},
		{"weak_password", ErrWeakPassword, TypeInvalidRequest, "weak_password", http.StatusBadRequest, nil},
		{"password_unchanged", ErrPasswordUnchanged, TypeInvalidRequest, "password_unchanged", http.StatusBadRequest, strPtr("new_password")},
		{"invalid_permission", ErrInvalidPermission, TypeInvalidRequest, "invalid_permission", http.StatusBadRequest, nil},
		{"invalid_credentials", ErrInvalidCredentials, TypeAuthentication, "invalid_credentials", http.StatusUnauthorized, nil},
		{"unauthorized", ErrUnauthorized, TypeAuthentication, "unauthorized", http.StatusUnauthorized, nil},
		{"forbidden", ErrForbidden, TypePermission, "forbidden", http.StatusForbidden, nil},
		{"grant_forbidden", ErrGrantForbidden, TypePermission, "grant_forbidden", http.StatusForbidden, nil},
		{"owner_protected", ErrOwnerProtected, TypePermission, "owner_protected", http.StatusForbidden, nil},
		{"cannot_modify_self", ErrCannotModifySelf, TypePermission, "cannot_modify_self", http.StatusForbidden, nil},
		{"registration_closed", ErrRegistrationClosed, TypePermission, "registration_closed", http.StatusForbidden, nil},
		{"not_found", ErrNotFound, TypeNotFound, "not_found", http.StatusNotFound, nil},
		{"preset_group_locked", ErrPresetGroupLocked, TypeConflict, "preset_group_locked", http.StatusConflict, nil},
		{"email_taken", ErrEmailTaken, TypeConflict, "email_taken", http.StatusConflict, strPtr("email")},
		{"nickname_taken", ErrNicknameTaken, TypeConflict, "nickname_taken", http.StatusConflict, strPtr("nickname")},
		{"already_taken", ErrAlreadyTaken, TypeConflict, "already_taken", http.StatusConflict, nil},
		{"group_name_taken", ErrGroupNameTaken, TypeConflict, "group_name_taken", http.StatusConflict, strPtr("name")},
		{"group_not_empty", ErrGroupNotEmpty, TypeConflict, "group_not_empty", http.StatusConflict, nil},
		{"last_owner", ErrLastOwner, TypeConflict, "last_owner", http.StatusConflict, nil},
		{"mail_delivery_failed", ErrMailDeliveryFailed, TypeServer, "mail_delivery_failed", http.StatusInternalServerError, nil},
		{"service_unavailable", ErrServiceUnavailable, TypeServer, "service_unavailable", http.StatusServiceUnavailable, nil},
		{"internal_error", ErrInternal, TypeServer, "internal_error", http.StatusInternalServerError, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rr)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			Write(ctx, tc.sentinel)
			if rr.Code != tc.wantCode2 {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantCode2)
			}
			var m map[string]map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
				t.Fatalf("decode: %v", err)
			}
			inner := m["error"]
			if inner["type"] != string(tc.wantType) {
				t.Fatalf("type = %v, want %v", inner["type"], tc.wantType)
			}
			if inner["code"] != tc.wantCode {
				t.Fatalf("code = %v, want %v", inner["code"], tc.wantCode)
			}
			if _, exists := inner["message"]; exists {
				t.Fatal("message field must not exist in the error envelope")
			}
			if tc.wantParam == nil {
				if inner["param"] != nil {
					t.Fatalf("param = %v, want null", inner["param"])
				}
			} else if inner["param"] != *tc.wantParam {
				t.Fatalf("param = %v, want %v", inner["param"], *tc.wantParam)
			}
		})
	}
}

func TestBodyUnknownErrorFallsBack(t *testing.T) {
	plain := Body(errors.New("boom"))
	if plain.Code != "internal_error" || plain.Type != TypeServer {
		t.Fatalf("plain fallback = %+v, want internal_error/server_error", plain)
	}
	wrapped := fmt.Errorf("handler: %w", errors.New("x"))
	if Body(wrapped).Code != "internal_error" {
		t.Fatalf("wrapped fallback = %+v, want internal_error", Body(wrapped))
	}
}

func TestWriteEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rr)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	Write(ctx, ErrEmailTaken)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	var m map[string]map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	inner, ok := m["error"]
	if !ok {
		t.Fatalf("body = %s, want top-level error object", rr.Body.String())
	}
	if inner["type"] != "conflict_error" || inner["code"] != "email_taken" || inner["param"] != "email" {
		t.Fatalf("inner = %v", inner)
	}
	if _, exists := inner["message"]; exists {
		t.Fatal("message field must not exist in the error envelope")
	}

	rr2 := httptest.NewRecorder()
	ctx2, _ := gin.CreateTestContext(rr2)
	ctx2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	Write(ctx2, ErrInvalidRequest)
	var m2 map[string]map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &m2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	p, exists := m2["error"]["param"]
	if !exists {
		t.Fatal("param field must exist in JSON")
	}
	if p != nil {
		t.Fatalf("param = %v, want JSON null", p)
	}
}

func TestAbortStopsChain(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rr)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	Abort(ctx, ErrForbidden)
	if !ctx.IsAborted() {
		t.Fatal("context must be aborted")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestWithParamOverrides(t *testing.T) {
	wrapped := WithParam(ErrWeakPassword, "new_password")
	if !errors.Is(wrapped, ErrWeakPassword) {
		t.Fatal("WithParam must keep errors.Is compatibility")
	}
	body := Body(wrapped)
	if body.Param == nil || *body.Param != "new_password" {
		t.Fatalf("param = %v, want new_password", body.Param)
	}
	if ErrWeakPassword.Param != nil {
		t.Fatalf("sentinel param mutated: %v", *ErrWeakPassword.Param)
	}

	twice := WithParam(WithParam(ErrWeakPassword, "password"), "new_password")
	if !errors.Is(twice, ErrWeakPassword) {
		t.Fatal("double WithParam must keep errors.Is compatibility")
	}
	if overridden := Body(twice); overridden.Param == nil || *overridden.Param != "new_password" {
		t.Fatalf("param = %v, want new_password (second override wins)", overridden.Param)
	}
}

func TestBodyCopyDoesNotAliasSentinelParam(t *testing.T) {
	body := Body(ErrEmailTaken)
	if body.Param == nil {
		t.Fatal("email_taken must carry its default param")
	}
	*body.Param = "hacked"
	if *ErrEmailTaken.Param != "email" {
		t.Fatalf("sentinel param polluted through Body copy: %q", *ErrEmailTaken.Param)
	}
}

func realConnectError(t *testing.T) error {
	t.Helper()

	db, err := sql.Open("pgx", "postgresql://nobody:nobody@127.0.0.1:1/nowhere?sslmode=disable")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.QueryContext(context.Background(), "select 1")
	if err == nil {
		t.Fatal("expected the unreachable database to fail")
	}
	var connectErr *pgconn.ConnectError
	if !errors.As(err, &connectErr) {
		t.Fatalf("fixture error = %T, want *pgconn.ConnectError", err)
	}
	return err
}

func TestDatabaseOutageMapsToServiceUnavailable(t *testing.T) {
	connectErr := realConnectError(t)
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantT    int
	}{
		{"connect error", connectErr, "service_unavailable", http.StatusServiceUnavailable},
		{"wrapped connect error", fmt.Errorf("ent: query failed: %w", connectErr), "service_unavailable", http.StatusServiceUnavailable},
		{"net op error", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}, "service_unavailable", http.StatusServiceUnavailable},
		{"driver bad conn", driver.ErrBadConn, "service_unavailable", http.StatusServiceUnavailable},
		{"unexpected eof", io.ErrUnexpectedEOF, "service_unavailable", http.StatusServiceUnavailable},
		{"closed connection", io.EOF, "service_unavailable", http.StatusServiceUnavailable},
		{"admin shutdown", &pgconn.PgError{Code: "57P01"}, "service_unavailable", http.StatusServiceUnavailable},
		{"cannot connect now", &pgconn.PgError{Code: "57P03"}, "service_unavailable", http.StatusServiceUnavailable},
		{"too many connections", &pgconn.PgError{Code: "53300"}, "service_unavailable", http.StatusServiceUnavailable},
		{"connection exception", &pgconn.PgError{Code: "08006"}, "service_unavailable", http.StatusServiceUnavailable},
		{"unique violation", &pgconn.PgError{Code: "23505"}, "internal_error", http.StatusInternalServerError},
		{"syntax error", &pgconn.PgError{Code: "42601"}, "internal_error", http.StatusInternalServerError},
		{"plain error", errors.New("boom"), "internal_error", http.StatusInternalServerError},
		{"typed sentinel", ErrInvalidCredentials, "invalid_credentials", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rr)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			Write(ctx, tc.err)

			if rr.Code != tc.wantT {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tc.wantT, rr.Body.String())
			}
			var m map[string]map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if m["error"]["code"] != tc.wantCode {
				t.Fatalf("code = %v, want %v", m["error"]["code"], tc.wantCode)
			}
		})
	}
}

func TestRegistryIntegrity(t *testing.T) {
	all := []*Error{
		ErrInvalidRequest, ErrWeakPassword, ErrPasswordUnchanged, ErrInvalidPermission,
		ErrMethodNotAllowed,
		ErrInvalidCredentials, ErrUnauthorized, ErrTwoFactorRequired, ErrTwoFactorInvalidCode,
		ErrTwoFactorLocked, ErrForbidden, ErrGrantForbidden,
		ErrOwnerProtected, ErrCannotModifySelf, ErrRegistrationClosed, ErrNotFound, ErrPresetGroupLocked,
		ErrEmailTaken, ErrNicknameTaken, ErrAlreadyTaken, ErrGroupNameTaken,
		ErrGroupNotEmpty, ErrLastOwner, ErrTwoFactorAlreadyEnabled, ErrTwoFactorNotSetup,
		ErrTwoFactorNotEnabled, ErrMailDeliveryFailed, ErrServiceUnavailable, ErrInternal,
	}
	if len(all) != 29 {
		t.Fatalf("sentinel count = %d, want 29", len(all))
	}
	if len(statusByCode) != 29 {
		t.Fatalf("statusByCode entries = %d, want 29", len(statusByCode))
	}
	statusTypes := map[int]Type{
		http.StatusBadRequest:          TypeInvalidRequest,
		http.StatusMethodNotAllowed:    TypeInvalidRequest,
		http.StatusUnauthorized:        TypeAuthentication,
		http.StatusForbidden:           TypePermission,
		http.StatusNotFound:            TypeNotFound,
		http.StatusConflict:            TypeConflict,
		http.StatusInternalServerError: TypeServer,
		http.StatusServiceUnavailable:  TypeServer,
	}
	seenCodes := make(map[string]struct{}, len(all))
	for _, sentinel := range all {
		status, ok := statusByCode[sentinel.Code]
		if !ok {
			t.Fatalf("code %q not registered in statusByCode; Body would silently degrade to internal_error", sentinel.Code)
		}
		if _, dup := seenCodes[sentinel.Code]; dup {
			t.Fatalf("code %q claimed by more than one sentinel", sentinel.Code)
		}
		seenCodes[sentinel.Code] = struct{}{}
		if want := statusTypes[status]; want == "" || sentinel.Type != want {
			t.Fatalf("code %q: type %q inconsistent with status %d (want %q)", sentinel.Code, sentinel.Type, status, want)
		}
	}
}
