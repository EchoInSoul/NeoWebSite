package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/mediaasset"
	"website/backend/internal/modules/access"
)

type mediaResponseBody struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	SizeBytes    int64  `json:"size_bytes"`
	OriginalName string `json:"original_name"`
}

type errorResponseBody struct {
	Error struct {
		Type  string  `json:"type"`
		Code  string  `json:"code"`
		Param *string `json:"param"`
	} `json:"error"`
}

type deleteResponseBody struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func newMediaTestApp(t *testing.T) (*httptest.Server, *database.Database, string) {
	t.Helper()

	db := testDB(t)
	root := t.TempDir()
	start := time.Now()
	t.Cleanup(func() {
		_, err := db.Client.MediaAsset.Delete().Where(mediaasset.CreatedAtGTE(start)).Exec(context.Background())
		if err != nil {
			t.Errorf("cleanup media rows: %v", err)
		}
	})

	gin.SetMode(gin.TestMode)
	sm := scs.New()
	sm.Lifetime = time.Hour

	engine := gin.New()
	engine.HandleMethodNotAllowed = true
	engine.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), guard.SessionUserIDKey, c.Query("user"))
		c.Status(http.StatusNoContent)
	})

	module, err := New(db, root)
	if err != nil {
		t.Fatalf("new module: %v", err)
	}
	api := engine.Group("/api/v1")
	protected := api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(access.New(db).Service, sm))
	module.RegisterProtected(protected)
	module.RegisterStorage(engine)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, db, root
}

func seedClient(t *testing.T, ts *httptest.Server, userID uuid.UUID) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Get(ts.URL + "/seed?user=" + userID.String())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp.Body.Close()
	return client
}

func multipartBody(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func uploadFile(t *testing.T, client *http.Client, baseURL, filename string, content []byte) *http.Response {
	t.Helper()

	body, contentType := multipartBody(t, "file", filename, content)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/media", body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func decodeUpload(t *testing.T, resp *http.Response) mediaResponseBody {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body mediaResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func decodeError(t *testing.T, resp *http.Response) errorResponseBody {
	t.Helper()
	defer resp.Body.Close()

	var body errorResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

func TestUploadStoresFileAndRecord(t *testing.T) {
	ts, db, root := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	if body.ID == "" {
		t.Fatal("id is empty")
	}
	if body.SizeBytes != 5 {
		t.Fatalf("size = %d, want 5", body.SizeBytes)
	}
	if body.OriginalName != "a.jpg" {
		t.Fatalf("original name = %q, want a.jpg", body.OriginalName)
	}
	if !strings.HasPrefix(body.URL, "/storage/") || !strings.HasSuffix(body.URL, ".jpg") {
		t.Fatalf("url = %q", body.URL)
	}

	key := strings.TrimPrefix(body.URL, "/storage/")
	written, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(key)))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(written) != "hello" {
		t.Fatalf("content = %q", written)
	}

	row, err := db.Client.MediaAsset.Get(context.Background(), uuid.MustParse(body.ID))
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if len(key) < 10 || key[:10] != row.CreatedAt.UTC().Format("2006/01/02") {
		t.Fatalf("key date %q does not match created_at %v", key, row.CreatedAt)
	}
}

func TestUploadRequiresOwnerOrAdmin(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyUser))

	resp := uploadFile(t, client, ts.URL, "a.jpg", []byte("x"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestUploadRequiresAuthentication(t *testing.T) {
	ts, _, _ := newMediaTestApp(t)

	resp := uploadFile(t, &http.Client{}, ts.URL, "a.jpg", []byte("x"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestUploadRejectsMissingPart(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body, contentType := multipartBody(t, "other", "a.jpg", []byte("x"))
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/media", body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	decoded := decodeError(t, resp)
	if resp.StatusCode != http.StatusBadRequest || decoded.Error.Code != "invalid_request" {
		t.Fatalf("status = %d, code = %q", resp.StatusCode, decoded.Error.Code)
	}
}

func TestUploadRejectsNonMultipart(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/media", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	decoded := decodeError(t, resp)
	if resp.StatusCode != http.StatusBadRequest || decoded.Error.Code != "invalid_request" {
		t.Fatalf("status = %d, code = %q", resp.StatusCode, decoded.Error.Code)
	}
}

func TestUploadAcceptsZip(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body := decodeUpload(t, uploadFile(t, client, ts.URL, "素材包.zip", []byte("PK")))
	if !strings.HasSuffix(body.URL, ".zip") {
		t.Fatalf("url = %q, want .zip suffix", body.URL)
	}
	if body.OriginalName != "素材包.zip" {
		t.Fatalf("original name = %q", body.OriginalName)
	}
}

func TestUploadAcceptsEmptyFile(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body := decodeUpload(t, uploadFile(t, client, ts.URL, "empty.jpg", nil))
	if body.SizeBytes != 0 {
		t.Fatalf("size = %d, want 0", body.SizeBytes)
	}
}

func TestConcurrentUploadsUseDistinctKeys(t *testing.T) {
	ts, db, root := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	const uploads = 16
	bodies := make([]*bytes.Buffer, uploads)
	types := make([]string, uploads)
	contents := make([][]byte, uploads)
	for i := 0; i < uploads; i++ {
		contents[i] = []byte(fmt.Sprintf("content-%02d", i))
		bodies[i], types[i] = multipartBody(t, "file", fmt.Sprintf("f%d.jpg", i), contents[i])
	}

	results := make([]*http.Response, uploads)
	errs := make([]error, uploads)
	var wg sync.WaitGroup
	for i := 0; i < uploads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/media", bodies[i])
			if err != nil {
				errs[i] = err
				return
			}
			req.Header.Set("Content-Type", types[i])
			results[i], errs[i] = client.Do(req)
		}(i)
	}
	wg.Wait()

	seen := make(map[string]struct{}, uploads)
	for i := 0; i < uploads; i++ {
		if errs[i] != nil {
			t.Fatalf("upload %d: %v", i, errs[i])
		}
		url := decodeUpload(t, results[i]).URL
		if _, ok := seen[url]; ok {
			t.Fatalf("duplicate url %q", url)
		}
		seen[url] = struct{}{}

		written, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(url, "/storage/"))))
		if err != nil {
			t.Fatalf("read file for %q: %v", url, err)
		}
		found := false
		for _, content := range contents {
			if string(written) == string(content) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("file for %q has unexpected content %q", url, written)
		}
	}
}

func TestDeleteRemovesFileAndRecord(t *testing.T) {
	ts, db, root := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))
	key := strings.TrimPrefix(uploaded.URL, "/storage/")

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/media/"+uploaded.ID, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var decoded deleteResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ID != uploaded.ID || !decoded.Deleted {
		t.Fatalf("body = %+v", decoded)
	}

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(key))); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
	if _, err := db.Client.MediaAsset.Get(context.Background(), uuid.MustParse(uploaded.ID)); err == nil {
		t.Fatal("record still present")
	}
}

func TestDeleteWithMalformedIDReturns404(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/media/not-a-uuid", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	decoded := decodeError(t, resp)
	if resp.StatusCode != http.StatusNotFound || decoded.Error.Code != "not_found" {
		t.Fatalf("status = %d, code = %q", resp.StatusCode, decoded.Error.Code)
	}
}

func storageRequest(t *testing.T, client *http.Client, method, url string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestStorageServesUploadedFile(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content-type = %q, want image/jpeg", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q", got)
	}
}

func TestStorageHeadHasNoBody(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	resp := storageRequest(t, http.DefaultClient, http.MethodHead, ts.URL+uploaded.URL, nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Fatalf("head body = %q, want empty", body)
	}
}

func TestStorageServesOpenEndedRange(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, map[string]string{"Range": "bytes=0-"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Length"); got != "5" {
		t.Fatalf("content-length = %q, want 5", got)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 0-4/5" {
		t.Fatalf("content-range = %q, want bytes 0-4/5", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q", got)
	}
}

func TestStorageRejectsRangeBeyondEnd(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, map[string]string{"Range": "bytes=999999999-"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
}

func TestStorageSends304OnIfModifiedSince(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	first := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, nil)
	lastModified := first.Header.Get("Last-Modified")
	first.Body.Close()
	if lastModified == "" {
		t.Fatal("first response has no Last-Modified")
	}

	second := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, map[string]string{"If-Modified-Since": lastModified})
	defer second.Body.Close()
	if second.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", second.StatusCode)
	}
	if got := second.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("cache-control = %q", got)
	}
}

func TestStorageForcesDownloadForHTML(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "x.html", []byte("<script>alert(1)</script>")))

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+uploaded.URL, nil)
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content-type = %q, want application/octet-stream", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment; filename=\"") {
		t.Fatalf("content-disposition = %q", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("x-content-type-options = %q", got)
	}
}

func TestStorageReturns404ForMalformedKey(t *testing.T) {
	ts, _, _ := newMediaTestApp(t)

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+"/storage/2026/", nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want empty", body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("x-content-type-options = %q", got)
	}
}

func TestStorageReturns404ForMissingFile(t *testing.T) {
	ts, _, _ := newMediaTestApp(t)
	missing := "/storage/2026/10/27/" + uuid.Must(uuid.NewV7()).String() + ".jpg"

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+missing, nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(string(body), "404 page not found") {
		t.Fatalf("body = %q", body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("x-content-type-options = %q", got)
	}
}

func TestStorageRedirectsBarePrefix(t *testing.T) {
	ts, _, _ := newMediaTestApp(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	resp := storageRequest(t, client, http.MethodGet, ts.URL+"/storage", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want 301", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); !strings.HasSuffix(got, "/storage/") {
		t.Fatalf("location = %q", got)
	}
}

func TestStorageRejectsPost(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))

	resp := storageRequest(t, http.DefaultClient, http.MethodPost, ts.URL+uploaded.URL, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	allow := resp.Header.Get("Allow")
	if !strings.Contains(allow, "GET") || !strings.Contains(allow, "HEAD") {
		t.Fatalf("allow = %q", allow)
	}
}

func TestStorageReturns404WhenFileMissingButRecordExists(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	ctx := context.Background()
	now := time.Now().UTC()
	key := now.Format("2006/01/02") + "/" + uuid.Must(uuid.NewV7()).String() + ".jpg"
	if _, err := db.Client.MediaAsset.Create().
		SetProvider("local").
		SetKey(key).
		SetSizeBytes(1).
		SetOriginalName("x.jpg").
		SetCreatedAt(now).
		Save(ctx); err != nil {
		t.Fatalf("insert: %v", err)
	}

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+"/storage/"+key, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("x-content-type-options = %q", got)
	}
}

func TestDeleteSucceedsWhenFileAlreadyGone(t *testing.T) {
	ts, db, root := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	uploaded := decodeUpload(t, uploadFile(t, client, ts.URL, "a.jpg", []byte("hello")))
	key := strings.TrimPrefix(uploaded.URL, "/storage/")

	if err := os.Remove(filepath.Join(root, filepath.FromSlash(key))); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/media/"+uploaded.ID, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if _, err := db.Client.MediaAsset.Get(context.Background(), uuid.MustParse(uploaded.ID)); err == nil {
		t.Fatal("record still present")
	}
}

func TestWriteServiceErrorMapsStorageErrorTo500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/x", func(ctx *gin.Context) {
		writeServiceError(ctx, fmt.Errorf("%w: %w", errStorage, io.ErrUnexpectedEOF))
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestUploadLowercasesExtension(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body := decodeUpload(t, uploadFile(t, client, ts.URL, "PHOTO.JPG", []byte("x")))
	if !strings.HasSuffix(body.URL, ".jpg") {
		t.Fatalf("url = %q, want .jpg suffix", body.URL)
	}
}

func TestUploadFallsBackToBinExtension(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))

	body := decodeUpload(t, uploadFile(t, client, ts.URL, "noext", []byte("x")))
	if !strings.HasSuffix(body.URL, ".bin") {
		t.Fatalf("url = %q, want .bin suffix", body.URL)
	}
}

func TestDeleteWithUnknownIDReturns404(t *testing.T) {
	ts, db, _ := newMediaTestApp(t)
	client := seedClient(t, ts, createTestUser(t, db, rbac.GroupKeyOwner))
	id := uuid.Must(uuid.NewV7())

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/media/"+id.String(), nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	decoded := decodeError(t, resp)
	if resp.StatusCode != http.StatusNotFound || decoded.Error.Code != "not_found" {
		t.Fatalf("status = %d, code = %q", resp.StatusCode, decoded.Error.Code)
	}
}

func TestStorageReturns404ForKeyWithoutExtension(t *testing.T) {
	ts, _, _ := newMediaTestApp(t)
	key := "/storage/2026/10/27/" + uuid.Must(uuid.NewV7()).String()

	resp := storageRequest(t, http.DefaultClient, http.MethodGet, ts.URL+key, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
}
