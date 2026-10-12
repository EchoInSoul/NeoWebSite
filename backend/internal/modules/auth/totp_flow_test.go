package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/recoverycode"
)

const testCodeHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var testTOTPKey = []byte("0123456789abcdef0123456789abcdef")

var recoveryCodePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func currentWindow() int64 {
	return totpNow().Unix() / totpPeriodSeconds
}

func loginOK(t *testing.T, client *http.Client, tsURL, identifier, password string) {
	t.Helper()

	resp := postJSON(t, client, tsURL+"/api/v1/auth/login", LoginRequest{Identifier: identifier, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
}

func post2FA(t *testing.T, client *http.Client, tsURL, path string, body any) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/auth/2fa/"+path, body)
}

func setup2FA(t *testing.T, client *http.Client, tsURL string) TwoFactorSetupResponse {
	t.Helper()

	resp := postRaw(t, client, tsURL+"/api/v1/auth/2fa/setup", "")
	return decodeJSON[TwoFactorSetupResponse](t, resp, http.StatusOK)
}

func enable2FA(t *testing.T, client *http.Client, tsURL, secret string) RecoveryCodesResponse {
	t.Helper()

	resp := post2FA(t, client, tsURL, "enable", TwoFactorCodeRequest{Code: codeAt(t, secret, currentWindow())})
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

	setup := setup2FA(t, client, ts.URL)
	if setup.Secret == "" {
		t.Fatal("setup returned empty secret")
	}
	if !strings.HasPrefix(setup.OTPAuthURL, "otpauth://totp/NeoBlog:") {
		t.Fatalf("otpauth_url = %q, want prefix otpauth://totp/NeoBlog:", setup.OTPAuthURL)
	}

	issued := enable2FA(t, client, ts.URL, setup.Secret)
	if len(issued.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("recovery codes = %d, want %d", len(issued.RecoveryCodes), recoveryCodesPerBatch)
	}
	for _, code := range issued.RecoveryCodes {
		if !recoveryCodePattern.MatchString(code) {
			t.Fatalf("code %q does not match 32 lowercase hex", code)
		}
	}

	if row := userRow(t, db, id); row.TotpEnabledAt == nil {
		t.Fatal("totp_enabled_at = nil after enable, want set")
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
	loginOK(t, client, ts.URL, u.Email, password)

	first := setup2FA(t, client, ts.URL)
	if row := userRow(t, db, id); row.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter = %d after setup, want nil", *row.TotpLastCounter)
	}

	second := setup2FA(t, client, ts.URL)
	if first.Secret == second.Secret {
		t.Fatal("second setup returned the same secret")
	}
	if row := userRow(t, db, id); row.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter = %d after overwrite, want nil", *row.TotpLastCounter)
	}

	stale := post2FA(t, client, ts.URL, "enable", TwoFactorCodeRequest{Code: codeAt(t, first.Secret, currentWindow())})
	assertErrorCode(t, stale, http.StatusUnauthorized, "two_factor_invalid_code")

	replacement := enable2FA(t, client, ts.URL, second.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)

	resp := postRaw(t, client, ts.URL+"/api/v1/auth/2fa/setup", "")
	assertErrorCode(t, resp, http.StatusConflict, "two_factor_already_enabled")
}

func TestTwoFactorEnableWithoutSetup(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorCodeRequest{Code: "123456"})
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

	setup := setup2FA(t, client, ts.URL)
	wrong := wrongCodesFor(t, setup.Secret, 1)[0]

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorCodeRequest{Code: wrong})
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

func TestTwoFactorSecretEncryptedAtRest(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	setup := setup2FA(t, client, ts.URL)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)

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

	setup := setup2FA(t, client, ts.URL)
	issued := enable2FA(t, client, ts.URL, setup.Secret)

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

func TestTwoFactorEnableRevokesOtherSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	clientA := testClient(t)
	clientB := testClient(t)
	loginOK(t, clientA, ts.URL, u.Email, password)
	loginOK(t, clientB, ts.URL, u.Email, password)

	setup := setup2FA(t, clientA, ts.URL)
	enable2FA(t, clientA, ts.URL, setup.Secret)

	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := clientB.Get(ts.URL + "/api/v1/auth/me")
		if err != nil {
			t.Fatalf("me B attempt %d: %v", attempt, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("client B attempt %d status = %d, want 401", attempt, resp.StatusCode)
		}
	}

	mine, err := clientA.Get(ts.URL + "/api/v1/auth/me")
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

	return postJSON(t, client, tsURL+"/api/v1/auth/login", LoginRequest{Identifier: identifier, Password: password})
}

func logoutOK(t *testing.T, client *http.Client, tsURL string) {
	t.Helper()

	resp := postRaw(t, client, tsURL+"/api/v1/auth/logout", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
}

func verify2FA(t *testing.T, client *http.Client, tsURL, code string) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/auth/2fa/verify", TwoFactorCodeRequest{Code: code})
}

func nextWindowCode(t *testing.T, secret string) string {
	t.Helper()

	return pinWindow(t, secret, currentWindow()+2)
}

func meStatus(t *testing.T, client *http.Client, tsURL string) int {
	t.Helper()

	resp, err := client.Get(tsURL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func meBody(t *testing.T, client *http.Client, tsURL string) MeResponse {
	t.Helper()

	resp, err := client.Get(tsURL + "/api/v1/auth/me")
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	issued := enable2FA(t, client, ts.URL, setup.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)
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

	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)

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

	setup := setup2FA(t, clientA, ts.URL)
	enable2FA(t, clientA, ts.URL, setup.Secret)

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
