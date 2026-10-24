package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/recoverycode"
	"website/backend/internal/modules/settings"
)

const testCodeHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testTOTPKey = []byte("0123456789abcdef0123456789abcdef")

var recoveryCodePattern = regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){3}$`)

func currentWindow() int64 {
	return totpNow().Unix() / totpPeriodSeconds
}

func loginOK(t *testing.T, client *http.Client, tsURL, identifier, password string) {
	t.Helper()

	resp := postJSON(t, client, tsURL+"/api/v1/sessions", LoginRequest{Identifier: identifier, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
}

func post2FA(t *testing.T, client *http.Client, tsURL, path string, body any) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/users/me/2fa/"+path, body)
}

func setup2FA(t *testing.T, client *http.Client, tsURL, password string) TwoFactorSetupResponse {
	t.Helper()

	resp := post2FA(t, client, tsURL, "setup", TwoFactorSetupRequest{Password: password})
	return decodeJSON[TwoFactorSetupResponse](t, resp, http.StatusOK)
}

func enable2FA(t *testing.T, client *http.Client, tsURL, secret, password string) RecoveryCodesResponse {
	t.Helper()

	resp := post2FA(t, client, tsURL, "enable", TwoFactorEnableRequest{Code: codeAt(t, secret, currentWindow()), Password: password})
	return decodeJSON[RecoveryCodesResponse](t, resp, http.StatusOK)
}

func decodeJSON[T any](t *testing.T, resp *http.Response, want int) T {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %T: %v", out, err)
	}
	return out
}

func errorEnvelope(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	envelope, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want nested object", body["error"])
	}
	return envelope
}

func assertErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
	t.Helper()

	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
	}
	if code := decodeError(t, resp); code != wantCode {
		t.Fatalf("code = %q, want %q", code, wantCode)
	}
}

func wrongCodesFor(t *testing.T, secret string, count int) []string {
	t.Helper()

	window := currentWindow()
	taken := make(map[string]struct{}, totpSkewWindows*2+1)
	for offset := -int64(totpSkewWindows); offset <= int64(totpSkewWindows); offset++ {
		taken[codeAt(t, secret, window+offset)] = struct{}{}
	}

	codes := make([]string, 0, count)
	for candidate := 0; len(codes) < count; candidate++ {
		code := fmt.Sprintf("%06d", candidate)
		if _, ok := taken[code]; ok {
			continue
		}
		codes = append(codes, code)
	}
	return codes
}

func userRow(t *testing.T, db *database.Database, id uuid.UUID) *ent.User {
	t.Helper()

	row, err := db.Client.User.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	return row
}

func recoveryCodeHashes(t *testing.T, db *database.Database, id uuid.UUID) []string {
	t.Helper()

	rows, err := db.Client.RecoveryCode.Query().Where(recoverycode.UserIDEQ(id)).All(context.Background())
	if err != nil {
		t.Fatalf("query recovery codes: %v", err)
	}
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		hashes = append(hashes, row.CodeHash)
	}
	return hashes
}

func TestConsumeRecoveryCodeConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	userID := uuid.MustParse(u.ID)
	repo := newRepository(db)

	if _, err := db.Client.RecoveryCode.Create().SetUserID(userID).SetCodeHash(testCodeHash).Save(ctx); err != nil {
		t.Fatalf("seed recovery code: %v", err)
	}

	results := make([]bool, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			consumed, err := repo.consumeRecoveryCode(ctx, userID, testCodeHash)
			if err != nil {
				t.Errorf("consume %d: %v", i, err)
				return
			}
			results[i] = consumed
		}(i)
	}
	wg.Wait()

	consumed := 0
	for _, ok := range results {
		if ok {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("consumed = %d, want exactly 1", consumed)
	}

	remaining, err := db.Client.RecoveryCode.Query().
		Where(recoverycode.UserIDEQ(userID), recoverycode.UsedAtIsNil()).
		Count(ctx)
	if err != nil {
		t.Fatalf("count unused codes: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("unused codes = %d, want 0", remaining)
	}
}

func TestClaimTOTPCounterConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	userID := uuid.MustParse(u.ID)
	repo := newRepository(db)

	results := make([]bool, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claimed, err := repo.claimTOTPCounter(ctx, userID, 100)
			if err != nil {
				t.Errorf("claim %d: %v", i, err)
				return
			}
			results[i] = claimed
		}(i)
	}
	wg.Wait()

	claimed := 0
	for _, ok := range results {
		if ok {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want exactly 1", claimed)
	}

	fetched, err := db.Client.User.Get(ctx, userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpLastCounter == nil || *fetched.TotpLastCounter != 100 {
		t.Fatalf("stored counter = %v, want 100", fetched.TotpLastCounter)
	}
}

func TestClaimTOTPCounterOrdering(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	userID := uuid.MustParse(u.ID)
	repo := newRepository(db)

	for _, tc := range []struct {
		counter int64
		want    bool
		kind    string
	}{
		{counter: 100, want: true, kind: "first claim from nil"},
		{counter: 99, want: false, kind: "lower than stored"},
		{counter: 100, want: false, kind: "equal to stored"},
		{counter: 101, want: true, kind: "higher than stored"},
	} {
		claimed, err := repo.claimTOTPCounter(ctx, userID, tc.counter)
		if err != nil {
			t.Fatalf("claim %d (%s): %v", tc.counter, tc.kind, err)
		}
		if claimed != tc.want {
			t.Fatalf("claim %d (%s) = %v, want %v", tc.counter, tc.kind, claimed, tc.want)
		}
	}

	fetched, err := db.Client.User.Get(ctx, userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpLastCounter == nil || *fetched.TotpLastCounter != 101 {
		t.Fatalf("stored counter = %v, want 101", fetched.TotpLastCounter)
	}

	missing, err := repo.claimTOTPCounter(ctx, uuid.New(), 500)
	if err != nil && !ent.IsNotFound(err) {
		t.Fatalf("claim for unknown user: %v", err)
	}
	if missing {
		t.Fatal("claim for unknown user = true, want false")
	}
}

func TestTwoFactorSetupEnableLifecycle(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	if setup.Secret == "" {
		t.Fatal("setup returned empty secret")
	}
	const issuer = "两步验证"
	prefix := "otpauth://totp/" + url.PathEscape(issuer) + ":"
	if !strings.HasPrefix(setup.OTPAuthURL, prefix) {
		t.Fatalf("otpauth_url = %q, want prefix %s", setup.OTPAuthURL, prefix)
	}

	issued := enable2FA(t, client, ts.URL, setup.Secret, password)
	if len(issued.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("recovery codes = %d, want %d", len(issued.RecoveryCodes), recoveryCodesPerBatch)
	}
	for _, code := range issued.RecoveryCodes {
		if !recoveryCodePattern.MatchString(code) {
			t.Fatalf("code %q is not 4 groups of 4 uppercase base32", code)
		}
	}

	row := userRow(t, db, id)
	if row.TotpEnabledAt == nil {
		t.Fatal("totp_enabled_at = nil after enable, want set")
	}
	if row.AuthVersion != 1 {
		t.Fatalf("auth_version = %d after enable, want 1", row.AuthVersion)
	}
	if status := meStatus(t, client, ts.URL); status != http.StatusOK {
		t.Fatalf("current session me = %d, want 200", status)
	}
	stored := recoveryCodeHashes(t, db, id)
	if len(stored) != recoveryCodesPerBatch {
		t.Fatalf("recovery code rows = %d, want %d", len(stored), recoveryCodesPerBatch)
	}
	for _, code := range issued.RecoveryCodes {
		if !containsHash(stored, hashRecoveryCode(code)) {
			t.Fatalf("code %q hash missing from storage", code)
		}
	}
}

func TestTwoFactorSetupOverwritesSecret(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	base := time.Unix(1700000001, 0)
	advance := fakeTOTPClock(t, base)
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL, password)
	if row := userRow(t, db, id); row.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter = %d after setup, want nil", *row.TotpLastCounter)
	}

	advance(base.Add(totpSetupReuseWindow + time.Second))
	second := setup2FA(t, client, ts.URL, password)
	if first.Secret == second.Secret {
		t.Fatal("second setup returned the same secret")
	}
	if row := userRow(t, db, id); row.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter = %d after overwrite, want nil", *row.TotpLastCounter)
	}

	stale := post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: codeAt(t, first.Secret, currentWindow()), Password: password})
	assertErrorCode(t, stale, http.StatusUnauthorized, "two_factor_invalid_code")

	replacement := enable2FA(t, client, ts.URL, second.Secret, password)
	if len(replacement.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("recovery codes = %d, want %d", len(replacement.RecoveryCodes), recoveryCodesPerBatch)
	}
}

func TestTwoFactorSetupRejectedWhenEnabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

	resp := post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password})
	assertErrorCode(t, resp, http.StatusConflict, "two_factor_already_enabled")
}

func TestTwoFactorEnableWithoutSetup(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: "123456", Password: password})
	assertErrorCode(t, resp, http.StatusConflict, "two_factor_not_setup")
}

func TestTwoFactorDisableWithoutSetup(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	resp := post2FA(t, client, ts.URL, "disable", TwoFactorDisableRequest{Password: password})
	assertErrorCode(t, resp, http.StatusConflict, "two_factor_not_setup")
}

func TestTwoFactorEnableWrongCode(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	wrong := wrongCodesFor(t, setup.Secret, 1)[0]

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: wrong, Password: password})
	if resp.StatusCode != http.StatusUnauthorized {
		resp.Body.Close()
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	envelope := errorEnvelope(t, resp)
	if envelope["code"] != "two_factor_invalid_code" {
		t.Fatalf("code = %v, want two_factor_invalid_code", envelope["code"])
	}
	if envelope["param"] != "code" {
		t.Fatalf("param = %v, want code", envelope["param"])
	}

	row := userRow(t, db, id)
	if row.TotpEnabledAt != nil {
		t.Fatal("totp_enabled_at set after wrong code, want nil")
	}
	if hashes := recoveryCodeHashes(t, db, id); len(hashes) != 0 {
		t.Fatalf("recovery code rows = %d after wrong code, want 0", len(hashes))
	}
}

func TestTwoFactorSetupRequiresPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	assertErrorCode(t, post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{}), http.StatusBadRequest, "invalid_request")
	assertErrorCode(t, post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password + "-wrong"}), http.StatusUnauthorized, "invalid_credentials")
	if row := userRow(t, db, id); row.TotpSecret != nil {
		t.Fatal("wrong password wrote a totp secret")
	}

	first := decodeJSON[TwoFactorSetupResponse](t, post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password}), http.StatusOK)
	if first.Secret == "" {
		t.Fatal("setup returned an empty secret")
	}
	before := userRow(t, db, id)
	if before.TotpSecret == nil {
		t.Fatal("setup did not persist a sealed secret")
	}

	assertErrorCode(t, post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password + "-wrong"}), http.StatusUnauthorized, "invalid_credentials")
	after := userRow(t, db, id)
	if after.TotpSecret == nil || *after.TotpSecret != *before.TotpSecret {
		t.Fatal("wrong password changed the stored totp secret")
	}
}

func TestTwoFactorEnableRequiresPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	code := codeAt(t, setup.Secret, currentWindow())

	assertErrorCode(t, post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: code}), http.StatusBadRequest, "invalid_request")
	assertErrorCode(t, post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: code, Password: password + "-wrong"}), http.StatusUnauthorized, "invalid_credentials")
	if row := userRow(t, db, id); row.TotpEnabledAt != nil {
		t.Fatal("wrong password enabled two-factor authentication")
	}
	if hashes := recoveryCodeHashes(t, db, id); len(hashes) != 0 {
		t.Fatalf("recovery code rows = %d after wrong password, want 0", len(hashes))
	}

	issued := decodeJSON[RecoveryCodesResponse](t, post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: code, Password: password}), http.StatusOK)
	if len(issued.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("recovery codes = %d, want %d", len(issued.RecoveryCodes), recoveryCodesPerBatch)
	}
}

func TestTwoFactorSecretEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	stored := userRow(t, db, id).TotpSecret
	if stored == nil {
		t.Fatal("totp_secret nil after setup")
	}
	if *stored == setup.Secret {
		t.Fatal("totp_secret stored as plaintext")
	}

	opened, err := openSecret(testTOTPKey, *stored)
	if err != nil {
		t.Fatalf("open stored secret: %v", err)
	}
	if opened != setup.Secret {
		t.Fatalf("opened secret = %q, want %q", opened, setup.Secret)
	}
}

func TestTwoFactorDisableLifecycle(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

	resp := post2FA(t, client, ts.URL, "disable", TwoFactorDisableRequest{Password: password})
	decodeJSON[map[string]any](t, resp, http.StatusOK)

	row := userRow(t, db, id)
	if row.TotpSecret != nil {
		t.Fatalf("totp_secret = %v after disable, want nil", *row.TotpSecret)
	}
	if row.TotpEnabledAt != nil {
		t.Fatalf("totp_enabled_at = %v after disable, want nil", *row.TotpEnabledAt)
	}
	if row.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter = %v after disable, want nil", *row.TotpLastCounter)
	}
	if row.TotpSetupAt != nil {
		t.Fatalf("totp_setup_at = %v after disable, want nil", *row.TotpSetupAt)
	}
	if hashes := recoveryCodeHashes(t, db, id); len(hashes) != 0 {
		t.Fatalf("recovery code rows = %d after disable, want 0", len(hashes))
	}
}

func TestTwoFactorRegenerateRecoveryCodes(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	notEnabled := post2FA(t, client, ts.URL, "recovery-codes", TwoFactorDisableRequest{Password: password})
	assertErrorCode(t, notEnabled, http.StatusConflict, "two_factor_not_enabled")

	setup := setup2FA(t, client, ts.URL, password)
	issued := enable2FA(t, client, ts.URL, setup.Secret, password)

	badPassword := post2FA(t, client, ts.URL, "recovery-codes", TwoFactorDisableRequest{Password: "wrong-password-1"})
	assertErrorCode(t, badPassword, http.StatusUnauthorized, "invalid_credentials")

	regen := post2FA(t, client, ts.URL, "recovery-codes", TwoFactorDisableRequest{Password: password})
	replacement := decodeJSON[RecoveryCodesResponse](t, regen, http.StatusOK)
	if len(replacement.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("regenerated codes = %d, want %d", len(replacement.RecoveryCodes), recoveryCodesPerBatch)
	}

	oldHashes := make(map[string]struct{}, len(issued.RecoveryCodes))
	for _, code := range issued.RecoveryCodes {
		oldHashes[hashRecoveryCode(code)] = struct{}{}
	}
	stored := recoveryCodeHashes(t, db, id)
	if len(stored) != recoveryCodesPerBatch {
		t.Fatalf("recovery code rows = %d, want %d", len(stored), recoveryCodesPerBatch)
	}
	for _, hash := range stored {
		if _, ok := oldHashes[hash]; ok {
			t.Fatal("old recovery code survived regeneration")
		}
	}
	for _, code := range replacement.RecoveryCodes {
		if _, ok := oldHashes[hashRecoveryCode(code)]; ok {
			t.Fatalf("regenerated code %q repeats an old one", code)
		}
	}
}

func TestEmailRecoveryCodesReserved(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)

	anonymous := post2FA(t, testClient(t), ts.URL, "recovery-codes/email", struct{}{})
	assertErrorCode(t, anonymous, http.StatusUnauthorized, "unauthorized")

	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	reserved := post2FA(t, client, ts.URL, "recovery-codes/email", struct{}{})
	assertErrorCode(t, reserved, http.StatusNotImplemented, "not_implemented")
}

func TestTwoFactorEnableRevokesOtherSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	clientA := testClient(t)
	clientB := testClient(t)
	loginOK(t, clientA, ts.URL, u.Email, password)
	loginOK(t, clientB, ts.URL, u.Email, password)

	setup := setup2FA(t, clientA, ts.URL, password)
	enable2FA(t, clientA, ts.URL, setup.Secret, password)

	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := clientB.Get(ts.URL + "/api/v1/users/me")
		if err != nil {
			t.Fatalf("me B attempt %d: %v", attempt, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("client B attempt %d status = %d, want 401", attempt, resp.StatusCode)
		}
	}

	mine, err := clientA.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me A: %v", err)
	}
	mine.Body.Close()
	if mine.StatusCode != http.StatusOK {
		t.Fatalf("client A status = %d, want 200 (current session must survive)", mine.StatusCode)
	}
}

func loginRaw(t *testing.T, client *http.Client, tsURL, identifier, password string) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/sessions", LoginRequest{Identifier: identifier, Password: password})
}

func logoutOK(t *testing.T, client *http.Client, tsURL string) {
	t.Helper()

	resp := doDELETE(t, client, tsURL+"/api/v1/sessions/current")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
}

func verify2FA(t *testing.T, client *http.Client, tsURL, code string) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/sessions/current/verify", TwoFactorCodeRequest{Code: code})
}

func nextWindowCode(t *testing.T, secret string) string {
	t.Helper()

	return pinWindow(t, secret, currentWindow()+2)
}

func meStatus(t *testing.T, client *http.Client, tsURL string) int {
	t.Helper()

	resp, err := client.Get(tsURL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func meBody(t *testing.T, client *http.Client, tsURL string) MeResponse {
	t.Helper()

	resp, err := client.Get(tsURL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	return decodeJSON[MeResponse](t, resp, http.StatusOK)
}

func unusedCodeCount(t *testing.T, db *database.Database, id uuid.UUID, code string) int {
	t.Helper()

	count, err := db.Client.RecoveryCode.Query().
		Where(
			recoverycode.UserIDEQ(id),
			recoverycode.CodeHashEQ(hashRecoveryCode(code)),
			recoverycode.UsedAtIsNil(),
		).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count unused codes: %v", err)
	}
	return count
}

func TestTwoFactorLoginSecondStageLifecycle(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)
	logoutOK(t, client, ts.URL)

	first := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, first, http.StatusUnauthorized, "two_factor_required")

	if status := meStatus(t, client, ts.URL); status != http.StatusUnauthorized {
		t.Fatalf("pending me status = %d, want 401", status)
	}

	wrong := verify2FA(t, client, ts.URL, wrongCodesFor(t, setup.Secret, 1)[0])
	assertErrorCode(t, wrong, http.StatusUnauthorized, "two_factor_invalid_code")

	code := nextWindowCode(t, setup.Secret)
	body := decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, code), http.StatusOK)
	if body.Email != u.Email {
		t.Fatalf("verify email = %q, want %q", body.Email, u.Email)
	}

	if me := meBody(t, client, ts.URL); !me.TwoFactorEnabled {
		t.Fatal("me two_factor_enabled = false, want true")
	}

	replay := verify2FA(t, client, ts.URL, code)
	assertErrorCode(t, replay, http.StatusUnauthorized, "unauthorized")
}

func TestTwoFactorLoginClearsExistingSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)
	if status := meStatus(t, client, ts.URL); status != http.StatusOK {
		t.Fatalf("me before relogin = %d, want 200", status)
	}

	relogin := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, relogin, http.StatusUnauthorized, "two_factor_required")

	if status := meStatus(t, client, ts.URL); status != http.StatusUnauthorized {
		t.Fatalf("me after entering pending = %d, want 401 (auth.user_id must be cleared)", status)
	}

	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, nextWindowCode(t, setup.Secret)), http.StatusOK)
	if status := meStatus(t, client, ts.URL); status != http.StatusOK {
		t.Fatalf("me after verify = %d, want 200", status)
	}
}

func TestTwoFactorRecoveryCodeLoginAndSingleUse(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	issued := enable2FA(t, client, ts.URL, setup.Secret, password)
	logoutOK(t, client, ts.URL)

	relogin := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, relogin, http.StatusUnauthorized, "two_factor_required")

	reused := "  " + strings.ToUpper(issued.RecoveryCodes[0]) + "  "
	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, reused), http.StatusOK)
	if count := unusedCodeCount(t, db, id, issued.RecoveryCodes[0]); count != 0 {
		t.Fatalf("unused rows for consumed code = %d, want 0", count)
	}

	logoutOK(t, client, ts.URL)
	third := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, third, http.StatusUnauthorized, "two_factor_required")

	secondUse := verify2FA(t, client, ts.URL, reused)
	assertErrorCode(t, secondUse, http.StatusUnauthorized, "two_factor_invalid_code")
}

func TestTwoFactorReplayRejected(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)
	logoutOK(t, client, ts.URL)

	code := nextWindowCode(t, setup.Secret)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, code), http.StatusOK)

	logoutOK(t, client, ts.URL)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
	replay := verify2FA(t, client, ts.URL, code)
	assertErrorCode(t, replay, http.StatusUnauthorized, "two_factor_invalid_code")
}

func TestTwoFactorVerifyLockout(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)
	logoutOK(t, client, ts.URL)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()

	code := nextWindowCode(t, setup.Secret)
	wrong := wrongCodesFor(t, setup.Secret, 5)

	for _, value := range wrong[:4] {
		resp := verify2FA(t, client, ts.URL, value)
		assertErrorCode(t, resp, http.StatusUnauthorized, "two_factor_invalid_code")
	}

	locked := verify2FA(t, client, ts.URL, wrong[4])
	assertErrorCode(t, locked, http.StatusUnauthorized, "two_factor_locked")

	freshSession := verify2FA(t, client, ts.URL, code)
	assertErrorCode(t, freshSession, http.StatusUnauthorized, "unauthorized")

	relogin := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, relogin, http.StatusUnauthorized, "two_factor_required")
	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, code), http.StatusOK)
}

func TestTwoFactorDecryptFailureIsServerError(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ctx := context.Background()
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

	claimed := userRow(t, db, id).TotpLastCounter
	if claimed == nil {
		t.Fatal("totp_last_counter nil after enable, want claimed window")
	}

	if err := db.Client.User.UpdateOneID(id).SetTotpSecret(setup.Secret).Exec(ctx); err != nil {
		t.Fatalf("corrupt secret column: %v", err)
	}

	logoutOK(t, client, ts.URL)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()

	code := nextWindowCode(t, setup.Secret)
	failed := verify2FA(t, client, ts.URL, code)
	assertErrorCode(t, failed, http.StatusInternalServerError, "internal_error")

	after := userRow(t, db, id).TotpLastCounter
	if after == nil || *after != *claimed {
		t.Fatalf("counter after decrypt failure = %v, want unchanged %v", after, *claimed)
	}

	sealed, err := sealSecret(testTOTPKey, setup.Secret)
	if err != nil {
		t.Fatalf("reseal secret: %v", err)
	}
	if err := db.Client.User.UpdateOneID(id).SetTotpSecret(sealed).Exec(ctx); err != nil {
		t.Fatalf("repair secret column: %v", err)
	}

	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, code), http.StatusOK)
}

func TestTwoFactorUnchangedForDisabledUsers(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := loginRaw(t, client, ts.URL, u.Email, password)
	decodeJSON[UserResponse](t, login, http.StatusOK)

	if me := meBody(t, client, ts.URL); me.TwoFactorEnabled {
		t.Fatal("me two_factor_enabled = true, want false for user without 2FA")
	}

	noPending := verify2FA(t, client, ts.URL, "123456")
	assertErrorCode(t, noPending, http.StatusUnauthorized, "unauthorized")
}

func TestTwoFactorDisableRevokesOtherSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	clientA := testClient(t)
	clientC := testClient(t)
	loginOK(t, clientA, ts.URL, u.Email, password)

	setup := setup2FA(t, clientA, ts.URL, password)
	enable2FA(t, clientA, ts.URL, setup.Secret, password)

	cLogin := loginRaw(t, clientC, ts.URL, u.Email, password)
	assertErrorCode(t, cLogin, http.StatusUnauthorized, "two_factor_required")
	decodeJSON[UserResponse](t, verify2FA(t, clientC, ts.URL, nextWindowCode(t, setup.Secret)), http.StatusOK)

	disable := post2FA(t, clientC, ts.URL, "disable", TwoFactorDisableRequest{Password: password})
	decodeJSON[map[string]any](t, disable, http.StatusOK)

	if status := meStatus(t, clientA, ts.URL); status != http.StatusUnauthorized {
		t.Fatalf("client A status = %d, want 401 after disable from another session", status)
	}
	if status := meStatus(t, clientC, ts.URL); status != http.StatusOK {
		t.Fatalf("client C status = %d, want 200 (current session must survive)", status)
	}
}

func containsHash(hashes []string, want string) bool {
	for _, hash := range hashes {
		if hash == want {
			return true
		}
	}
	return false
}

func fakeTOTPClock(t *testing.T, base time.Time) func(time.Time) {
	t.Helper()
	previous := totpNow
	now := base
	totpNow = func() time.Time { return now }
	t.Cleanup(func() { totpNow = previous })
	return func(at time.Time) { now = at }
}

func TestClaimTOTPSetupRejectedWhenFresh(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	userID := uuid.MustParse(u.ID)
	repo := newRepository(db)
	base := time.Unix(1700000001, 0)

	claimed, err := repo.claimTOTPSetup(ctx, userID, testCodeHash, base)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim on fresh user rejected, want success")
	}

	reclaimed, err := repo.claimTOTPSetup(ctx, userID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", base)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if reclaimed {
		t.Fatal("second claim inside reuse window accepted, want rejected")
	}

	stale, err := repo.claimTOTPSetup(ctx, userID, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", base.Add(totpSetupReuseWindow+time.Second))
	if err != nil {
		t.Fatalf("stale claim: %v", err)
	}
	if !stale {
		t.Fatal("claim past reuse window rejected, want success")
	}
}

func TestTwoFactorSetupReusesSecretImmediately(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL, password)
	before := userRow(t, db, id)
	second := setup2FA(t, client, ts.URL, password)
	after := userRow(t, db, id)

	if second.Secret != first.Secret || second.OTPAuthURL != first.OTPAuthURL {
		t.Fatalf("reuse changed response: secret %v url %v", second.Secret != first.Secret, second.OTPAuthURL != first.OTPAuthURL)
	}
	if *after.TotpSecret != *before.TotpSecret {
		t.Fatal("reuse rewrote totp_secret, want zero writes")
	}
	if after.TotpSetupAt == nil || !after.TotpSetupAt.Equal(*before.TotpSetupAt) {
		t.Fatalf("totp_setup_at changed on reuse: %v -> %v", before.TotpSetupAt, after.TotpSetupAt)
	}
	enable2FA(t, client, ts.URL, first.Secret, password)
}

func TestTwoFactorSetupReusesSecretJustInsideWindow(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	window := totpSetupReuseWindow
	advance := fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL, password)
	advance(time.Unix(1700000001, 0).Add(window - time.Second))
	second := setup2FA(t, client, ts.URL, password)

	if second.Secret != first.Secret {
		t.Fatalf("secret changed inside window: %q != %q", second.Secret, first.Secret)
	}
}

func TestTwoFactorSetupRegeneratesAtWindowBoundary(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	base := time.Unix(1700000001, 0)
	window := totpSetupReuseWindow
	advance := fakeTOTPClock(t, base)
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL, password)
	advance(base.Add(window))
	second := setup2FA(t, client, ts.URL, password)

	if second.Secret == first.Secret {
		t.Fatal("secret unchanged at window boundary, want regeneration")
	}
	row := userRow(t, db, id)
	if row.TotpSetupAt == nil || !row.TotpSetupAt.Equal(base.Add(window)) {
		t.Fatalf("totp_setup_at = %v, want %v", row.TotpSetupAt, base.Add(window))
	}

	stale := post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: codeAt(t, first.Secret, currentWindow()), Password: password})
	assertErrorCode(t, stale, http.StatusUnauthorized, "two_factor_invalid_code")

	enable2FA(t, client, ts.URL, second.Secret, password)
}

func TestTwoFactorSetupConcurrentCallsShareSecret(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	svc := newService(db, scs.New(), testTOTPKey, settingsModule.Service, ratelimit.New(), disabledRateLimitPolicies(), nil)

	results := make([]TwoFactorSetupResponse, 8)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.SetupTOTP(context.Background(), u.ID, password)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("setup %d: %v", i, err)
		}
	}
	for i, r := range results {
		if r.Secret != results[0].Secret {
			t.Fatalf("setup %d secret %q != %q, want one shared secret", i, r.Secret, results[0].Secret)
		}
	}
	sealed := userRow(t, db, id).TotpSecret
	if sealed == nil {
		t.Fatal("totp_secret nil after concurrent setup")
	}
	plain, err := openSecret(testTOTPKey, *sealed)
	if err != nil {
		t.Fatalf("open stored secret: %v", err)
	}
	if plain != results[0].Secret {
		t.Fatalf("stored secret %q != response %q", plain, results[0].Secret)
	}
}

func TestTwoFactorSetupAtClearedByDisable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	setup2FA(t, client, ts.URL, password)
	if row := userRow(t, db, id); row.TotpSetupAt == nil {
		t.Fatal("totp_setup_at nil after setup, precondition broken")
	}

	resp := post2FA(t, client, ts.URL, "disable", TwoFactorDisableRequest{Password: password})
	decodeJSON[map[string]any](t, resp, http.StatusOK)

	row := userRow(t, db, id)
	if row.TotpSetupAt != nil {
		t.Fatalf("totp_setup_at = %v after disable, want nil", *row.TotpSetupAt)
	}
	if row.TotpSecret != nil {
		t.Fatalf("totp_secret = %v after disable, want nil", *row.TotpSecret)
	}
}

func TestTwoFactorSetupAtKeptByEnable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	setup2FA(t, client, ts.URL, password)
	before := userRow(t, db, id).TotpSetupAt
	if before == nil {
		t.Fatal("totp_setup_at nil after setup, precondition broken")
	}

	enable2FA(t, client, ts.URL, userRowSecret(t, db, id), password)

	if after := userRow(t, db, id).TotpSetupAt; after == nil || !after.Equal(*before) {
		t.Fatalf("totp_setup_at = %v after enable, want unchanged %v", after, *before)
	}
}

func userRowSecret(t *testing.T, db *database.Database, id uuid.UUID) string {
	t.Helper()

	row := userRow(t, db, id)
	if row.TotpSecret == nil {
		t.Fatal("totp_secret nil, want sealed secret")
	}
	plain, err := openSecret(testTOTPKey, *row.TotpSecret)
	if err != nil {
		t.Fatalf("open stored secret: %v", err)
	}
	return plain
}

func TestTwoFactorSetupLegacyPendingWithoutSetupAt(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ctx := context.Background()
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL, password)
	if err := db.Client.User.UpdateOneID(id).ClearTotpSetupAt().Exec(ctx); err != nil {
		t.Fatalf("clear setup_at: %v", err)
	}

	second := setup2FA(t, client, ts.URL, password)

	if second.Secret == first.Secret {
		t.Fatal("legacy pending row reused old secret, want regeneration")
	}
	if row := userRow(t, db, id); row.TotpSetupAt == nil {
		t.Fatal("totp_setup_at still nil after legacy regeneration")
	}
}

func TestTwoFactorSetupReuseDecryptFailureIsServerError(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ctx := context.Background()
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL, password)
	if err := db.Client.User.UpdateOneID(id).SetTotpSecret(setup.Secret).Exec(ctx); err != nil {
		t.Fatalf("corrupt secret column: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password})
	assertErrorCode(t, resp, http.StatusInternalServerError, "internal_error")

	row := userRow(t, db, id)
	if row.TotpSecret == nil || *row.TotpSecret != setup.Secret {
		t.Fatalf("totp_secret = %v, want corrupted value left in place", row.TotpSecret)
	}
}

func TestTwoFactorSetupReuseEmptySecretIsServerError(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ctx := context.Background()
	ts := newTestApp(t, db)
	client := testClient(t)
	fakeTOTPClock(t, time.Unix(1700000001, 0))
	loginOK(t, client, ts.URL, u.Email, password)

	setup2FA(t, client, ts.URL, password)
	sealed, err := sealSecret(testTOTPKey, "")
	if err != nil {
		t.Fatalf("seal empty secret: %v", err)
	}
	if err := db.Client.User.UpdateOneID(id).SetTotpSecret(sealed).Exec(ctx); err != nil {
		t.Fatalf("seed empty plaintext: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password})
	assertErrorCode(t, resp, http.StatusInternalServerError, "internal_error")

	if row := userRow(t, db, id); row.TotpSecret == nil || *row.TotpSecret != sealed {
		t.Fatalf("totp_secret = %v, want empty plaintext left in place", row.TotpSecret)
	}
}
