package api

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docpocket/docpocket/internal/auth"
	"github.com/docpocket/docpocket/internal/collections"
	"github.com/docpocket/docpocket/internal/files"
	"github.com/docpocket/docpocket/internal/realtime"
	"github.com/docpocket/docpocket/internal/rules"
	"github.com/docpocket/docpocket/internal/storage"
	"github.com/docpocket/docpocket/ui"
)

type pipeListener struct {
	conns   chan net.Conn
	closed  atomic.Bool
	closeMu sync.Mutex
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn, 128)}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	c, ok := <-l.conns
	if !ok {
		return nil, net.ErrClosed
	}
	return c, nil
}

func (l *pipeListener) Close() error {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed.CompareAndSwap(false, true) {
		close(l.conns)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80}
}

func (l *pipeListener) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if l.closed.Load() {
		return nil, net.ErrClosed
	}
	s, c := net.Pipe()
	select {
	case l.conns <- s:
		return c, nil
	case <-ctx.Done():
		s.Close()
		c.Close()
		return nil, ctx.Err()
	}
}

type testAPI struct {
	t        *testing.T
	srv      *Server
	store    *storage.BoltStore
	auth     *auth.Service
	client   *http.Client
	listener *pipeListener
	token    string
	baseURL  string
}

func newTestAPI(t *testing.T) *testAPI {
	return newTestAPIWithHooks(t, Hooks{})
}

func newTestAPIWithHooks(t *testing.T, hooks Hooks) *testAPI {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.Open(filepath.Join(dir, "data"), storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := auth.New(store, auth.Config{
		Params: auth.Argon2Params{Memory: 8 << 10, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	ruleEngine, err := rules.New()
	if err != nil {
		t.Fatal(err)
	}
	fileStore, err := files.New(filepath.Join(dir, "files"), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Deps{
		Store:   store,
		Auth:    authSvc,
		Rules:   ruleEngine,
		Files:   fileStore,
		Hub:     realtime.NewHub(64, 32),
		Hooks:   hooks,
		UI:      ui.FS(),
		Version: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	listener := newPipeListener()
	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(listener)

	transport := &http.Transport{
		DialContext: listener.DialContext,
	}
	api := &testAPI{
		t:        t,
		srv:      srv,
		store:    store,
		auth:     authSvc,
		listener: listener,
		client:   &http.Client{Timeout: 15 * time.Second, Transport: transport},
		baseURL:  "http://docpocket.local",
	}
	t.Cleanup(func() {
		httpSrv.Close()
		listener.Close()
		srv.Close()
		_ = store.Close()
	})
	return api
}

func (a *testAPI) request(method, path string, body []byte, token, contentType string) *http.Response {
	a.t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, a.baseURL+path, r)
	if err != nil {
		a.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	return resp
}

func (a *testAPI) json(method, path string, body any, token string, want int) map[string]any {
	a.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
	}
	resp := a.request(method, path, raw, token, "application/json")
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if resp.StatusCode != want {
		a.t.Fatalf("%s %s: status %d, want %d, body: %s", method, path, resp.StatusCode, want, data)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		a.t.Fatalf("%s %s: invalid JSON %q: %v", method, path, data, err)
	}
	return out
}

func (a *testAPI) setupAdmin() {
	a.t.Helper()
	a.json("GET", "/api/admin/setup", nil, "", http.StatusOK)
	created := a.json("POST", "/api/admin/setup", map[string]any{
		"email": "admin@example.com", "password": "password-123",
	}, "", http.StatusCreated)
	if created["email"] != "admin@example.com" {
		a.t.Fatalf("unexpected setup response: %v", created)
	}
	session := a.json("POST", "/api/admin/auth-with-password", map[string]any{
		"email": "admin@example.com", "password": "password-123",
	}, "", http.StatusOK)
	token, _ := session["token"].(string)
	if token == "" {
		a.t.Fatalf("missing admin token: %v", session)
	}
	a.token = token
}

func (a *testAPI) createCollection(def map[string]any) map[string]any {
	a.t.Helper()
	return a.json("POST", "/api/admin/collections", def, a.token, http.StatusCreated)
}

func publicBaseCollection(name string, indexes ...map[string]any) map[string]any {
	def := map[string]any{
		"name": name,
		"type": "base",
		"rules": map[string]any{
			"list": "", "view": "", "create": "", "update": "", "delete": "",
		},
	}
	if len(indexes) > 0 {
		def["indexes"] = indexes
	}
	return def
}

func TestHealthAndAdminSetup(t *testing.T) {
	api := newTestAPI(t)

	health := api.json("GET", "/api/health", nil, "", http.StatusOK)
	if health["status"] != "ok" || health["version"] != "test" {
		t.Fatalf("unexpected health: %v", health)
	}
	status := api.json("GET", "/api/admin/setup", nil, "", http.StatusOK)
	if status["configured"] != false {
		t.Fatalf("expected unconfigured setup: %v", status)
	}
	api.setupAdmin()

	status = api.json("GET", "/api/admin/setup", nil, "", http.StatusOK)
	if status["configured"] != true {
		t.Fatalf("expected configured setup: %v", status)
	}
	api.json("POST", "/api/admin/setup", map[string]any{
		"email": "other@example.com", "password": "password-123",
	}, "", http.StatusConflict)
	api.json("POST", "/api/admin/auth-with-password", map[string]any{
		"email": "admin@example.com", "password": "wrong-password",
	}, "", http.StatusUnauthorized)
	me := api.json("GET", "/api/admin/me", nil, api.token, http.StatusOK)
	if me["kind"] != "admin" || me["email"] != "admin@example.com" {
		t.Fatalf("unexpected admin identity: %v", me)
	}
	api.json("GET", "/api/admin/me", nil, "", http.StatusUnauthorized)
}

func TestGzipJSONResponses(t *testing.T) {
	api := newTestAPI(t)
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{DisableCompression: true, DialContext: api.listener.DialContext},
	}

	plainReq, err := http.NewRequest("GET", api.baseURL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := client.Do(plainReq)
	if err != nil {
		t.Fatal(err)
	}
	plainBody, _ := io.ReadAll(plain.Body)
	plain.Body.Close()
	if plain.StatusCode != http.StatusOK || plain.Header.Get("Content-Encoding") != "" {
		t.Fatalf("unexpected plain response: %d %q", plain.StatusCode, plain.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(string(plainBody), `"status":"ok"`) {
		t.Fatalf("unexpected plain body: %s", plainBody)
	}

	gzipReq, err := http.NewRequest("GET", api.baseURL+"/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	gzipReq.Header.Set("Accept-Encoding", "gzip")
	compressed, err := client.Do(gzipReq)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Body.Close()
	if compressed.StatusCode != http.StatusOK || compressed.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip response, got %d %q", compressed.StatusCode, compressed.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var health map[string]any
	if err := json.NewDecoder(zr).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ok" {
		t.Fatalf("unexpected compressed payload: %v", health)
	}
}

func TestCollectionRecordCRUD(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()

	def := publicBaseCollection("posts",
		map[string]any{"name": "views_idx", "field": "views"},
		map[string]any{"name": "slug_unique", "field": "slug", "unique": true},
	)
	createdDef := api.createCollection(def)
	if createdDef["name"] != "posts" {
		t.Fatalf("unexpected collection: %v", createdDef)
	}
	indexes, _ := createdDef["indexes"].([]any)
	for _, raw := range indexes {
		idx := raw.(map[string]any)
		if idx["state"] != "ready" {
			t.Fatalf("index %v was not built: %v", idx["name"], idx)
		}
	}

	first := api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "hello", "views": 10, "slug": "hello",
	}, "", http.StatusCreated)
	id, _ := first["id"].(string)
	if id == "" || first["created"] == nil || first["updated"] == nil {
		t.Fatalf("system fields missing: %v", first)
	}
	api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "other", "views": 3, "slug": "hello",
	}, "", http.StatusConflict)
	second := api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "second", "views": 20, "slug": "second",
	}, "", http.StatusCreated)

	list := api.json("GET", "/api/collections/posts/records?filter=views%20%3E%205&sort=-views&limit=1", nil, "", http.StatusOK)
	items, _ := list["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "second" {
		t.Fatalf("unexpected list: %v", list)
	}
	if list["nextCursor"] == nil {
		t.Fatalf("expected next cursor: %v", list)
	}

	got := api.json("GET", "/api/collections/posts/records/"+id, nil, "", http.StatusOK)
	if got["title"] != "hello" {
		t.Fatalf("unexpected record: %v", got)
	}
	patched := api.json("PATCH", "/api/collections/posts/records/"+id, map[string]any{
		"title": "patched", "views": 11,
	}, "", http.StatusOK)
	if patched["title"] != "patched" {
		t.Fatalf("patch failed: %v", patched)
	}
	replaced := api.json("PUT", "/api/collections/posts/records/"+id, map[string]any{
		"title": "replaced", "views": 12, "slug": "hello",
	}, "", http.StatusOK)
	if replaced["created"] != first["created"] || replaced["title"] != "replaced" {
		t.Fatalf("replace did not preserve created: %v", replaced)
	}
	api.json("GET", "/api/collections/posts/records?filter="+",", nil, "", http.StatusBadRequest)
	api.json("GET", "/api/collections/posts/records?filter=views%20%3E", nil, "", http.StatusBadRequest)

	batch := api.json("POST", "/api/batch", map[string]any{
		"operations": []any{
			map[string]any{"method": "POST", "collection": "posts", "record": map[string]any{"title": "batch", "views": 1, "slug": "batch"}},
			map[string]any{"method": "DELETE", "collection": "posts", "id": second["id"]},
		},
	}, "", http.StatusOK)
	results, _ := batch["results"].([]any)
	if len(results) != 2 || results[1].(map[string]any)["deleted"] != true {
		t.Fatalf("unexpected batch: %v", batch)
	}

	collectionsList := api.json("GET", "/api/admin/collections", nil, api.token, http.StatusOK)
	cols, _ := collectionsList["items"].([]any)
	if len(cols) != 1 || cols[0].(map[string]any)["documentCount"] != json.Number("2") {
		t.Fatalf("unexpected admin collections: %v", collectionsList)
	}
	reindexed := api.json("POST", "/api/admin/collections/posts/reindex", nil, api.token, http.StatusOK)
	if reindexed["name"] != "posts" {
		t.Fatalf("unexpected reindex: %v", reindexed)
	}
	check := api.json("POST", "/api/admin/check", nil, api.token, http.StatusOK)
	issues, _ := check["issues"].([]any)
	if len(issues) != 0 {
		t.Fatalf("check found issues: %v", check)
	}

	api.json("DELETE", "/api/collections/posts/records/"+id, nil, "", http.StatusNoContent)
	api.json("GET", "/api/collections/posts/records/"+id, nil, "", http.StatusNotFound)
	api.json("DELETE", "/api/admin/collections/posts", nil, api.token, http.StatusBadRequest)
	api.json("DELETE", "/api/admin/collections/posts?confirm=posts", nil, api.token, http.StatusNoContent)
	api.json("GET", "/api/admin/collections/posts", nil, api.token, http.StatusNotFound)
}

func TestAuthCollectionFlow(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()

	def := map[string]any{
		"name": "users",
		"type": "auth",
		"fields": []any{
			map[string]any{"name": "email", "type": "string", "required": true},
			map[string]any{"name": "name", "type": "string"},
		},
	}
	api.createCollection(def)

	created := api.json("POST", "/api/collections/users/auth-register", map[string]any{
		"email": "ana@example.com", "password": "password-123", "name": "Ana",
	}, "", http.StatusCreated)
	token, _ := created["token"].(string)
	refresh, _ := created["refreshToken"].(string)
	record, _ := created["record"].(map[string]any)
	if token == "" || refresh == "" || record["name"] != "Ana" {
		t.Fatalf("unexpected registration: %v", created)
	}
	if record["passwordHash"] != nil || record["tokenKey"] != nil {
		t.Fatalf("secrets leaked: %v", record)
	}
	api.json("POST", "/api/collections/users/auth-register", map[string]any{
		"email": "ana@example.com", "password": "password-123",
	}, "", http.StatusConflict)
	api.json("POST", "/api/collections/users/auth-register", map[string]any{
		"email": "bad-email", "password": "password-123",
	}, "", http.StatusUnprocessableEntity)
	api.json("POST", "/api/collections/users/auth-register", map[string]any{
		"email": "weak@example.com", "password": "short",
	}, "", http.StatusUnprocessableEntity)

	login := api.json("POST", "/api/collections/users/auth-with-password", map[string]any{
		"identity": "ana@example.com", "password": "password-123",
	}, "", http.StatusOK)
	loginToken, _ := login["token"].(string)
	api.json("POST", "/api/collections/users/auth-with-password", map[string]any{
		"identity": "ana@example.com", "password": "wrong-password",
	}, "", http.StatusUnauthorized)

	me := api.json("GET", "/api/auth/me", nil, loginToken, http.StatusOK)
	if me["email"] != "ana@example.com" {
		t.Fatalf("unexpected me: %v", me)
	}
	own := api.json("GET", "/api/collections/users/records", nil, loginToken, http.StatusOK)
	items, _ := own["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["email"] != "ana@example.com" {
		t.Fatalf("owner list should see only their record: %v", own)
	}
	anonymous := api.json("GET", "/api/collections/users/records", nil, "", http.StatusOK)
	if anonItems, _ := anonymous["items"].([]any); len(anonItems) != 0 {
		t.Fatalf("anonymous list should be empty: %v", anonymous)
	}

	// Regular users cannot bypass the auth registration endpoint.
	api.json("POST", "/api/collections/users/records", map[string]any{"email": "x@example.com"}, loginToken, http.StatusForbidden)
	// A superuser can provision auth users with a password.
	provisioned := api.json("POST", "/api/collections/users/records", map[string]any{
		"email": "beto@example.com", "password": "password-123", "name": "Beto",
	}, api.token, http.StatusCreated)
	if provisioned["email"] != "beto@example.com" || provisioned["name"] != "Beto" {
		t.Fatalf("unexpected provisioned user: %v", provisioned)
	}
	if provisioned["passwordHash"] != nil || provisioned["tokenKey"] != nil {
		t.Fatalf("provisioned user leaked secrets: %v", provisioned)
	}
	api.json("POST", "/api/collections/users/records", map[string]any{
		"email": "nopassword@example.com",
	}, api.token, http.StatusUnprocessableEntity)
	api.json("POST", "/api/collections/users/auth-with-password", map[string]any{
		"identity": "beto@example.com", "password": "password-123",
	}, "", http.StatusOK)
	// A regular user cannot use the admin API.
	api.json("GET", "/api/admin/collections", nil, loginToken, http.StatusForbidden)

	refreshed := api.json("POST", "/api/collections/users/auth-refresh", map[string]any{
		"refreshToken": refresh,
	}, "", http.StatusOK)
	newRefresh, _ := refreshed["refreshToken"].(string)
	if newRefresh == refresh {
		t.Fatal("refresh token was not rotated")
	}
	api.json("POST", "/api/collections/users/auth-refresh", map[string]any{
		"refreshToken": refresh,
	}, "", http.StatusUnauthorized)
	api.json("POST", "/api/collections/users/auth-logout", map[string]any{
		"refreshToken": newRefresh,
	}, "", http.StatusNoContent)
	api.json("POST", "/api/collections/users/auth-refresh", map[string]any{
		"refreshToken": newRefresh,
	}, "", http.StatusUnauthorized)
}

func TestFileUploadDownloadDelete(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))
	doc := api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "with file",
	}, "", http.StatusCreated)
	id := doc["id"].(string)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello docpocket")); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("field", "attachment"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp := api.request("POST", "/api/collections/posts/records/"+id+"/files", buf.Bytes(), "", mw.FormDataContentType())
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status %d: %s", resp.StatusCode, raw)
	}
	var uploaded map[string]any
	if err := json.Unmarshal(raw, &uploaded); err != nil {
		t.Fatal(err)
	}
	name, _ := uploaded["name"].(string)
	if name == "" || uploaded["originalName"] != "notes.txt" {
		t.Fatalf("unexpected upload: %v", uploaded)
	}

	got := api.json("GET", "/api/collections/posts/records/"+id, nil, "", http.StatusOK)
	if got["attachment"] != name {
		t.Fatalf("file field not linked: %v", got)
	}
	download := api.request("GET", "/api/files/posts/"+id+"/"+name, nil, "", "")
	body, _ := io.ReadAll(download.Body)
	download.Body.Close()
	if download.StatusCode != http.StatusOK || string(body) != "hello docpocket" {
		t.Fatalf("download status %d body %q", download.StatusCode, body)
	}
	if !strings.Contains(download.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("unexpected content type: %q", download.Header.Get("Content-Type"))
	}
	api.json("DELETE", "/api/collections/posts/records/"+id+"/files/"+name+"?field=attachment", nil, "", http.StatusNoContent)
	got = api.json("GET", "/api/collections/posts/records/"+id, nil, "", http.StatusOK)
	if _, ok := got["attachment"]; ok {
		t.Fatalf("attachment field was not cleared: %v", got)
	}
	download = api.request("GET", "/api/files/posts/"+id+"/"+name, nil, "", "")
	download.Body.Close()
	if download.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", download.StatusCode)
	}
}

func TestRealtimeStreamAndSubscription(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", api.baseURL+"/api/realtime", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamClient := &http.Client{
		Transport: &http.Transport{DialContext: api.listener.DialContext},
	}
	resp, err := streamClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected content type: %q", resp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(resp.Body)
	event, _, data := readSSE(t, reader)
	if event != "connect" {
		t.Fatalf("expected connect event, got %q (%s)", event, data)
	}
	var connectPayload map[string]any
	if err := json.Unmarshal([]byte(data), &connectPayload); err != nil {
		t.Fatal(err)
	}
	clientID, _ := connectPayload["clientId"].(string)
	if clientID == "" {
		t.Fatalf("missing clientId: %s", data)
	}
	api.json("POST", "/api/realtime", map[string]any{
		"clientId":      clientID,
		"subscriptions": []string{"posts"},
	}, "", http.StatusNoContent)
	api.json("POST", "/api/realtime", map[string]any{
		"clientId":      "missing",
		"subscriptions": []string{"posts"},
	}, "", http.StatusNotFound)

	created := api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "live",
	}, "", http.StatusCreated)
	event, _, data = readSSE(t, reader)
	if event != "posts" {
		t.Fatalf("expected posts event, got %q (%s)", event, data)
	}
	var ev struct {
		Action string         `json:"action"`
		ID     string         `json:"id"`
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Action != "create" || ev.ID != created["id"] || ev.Record["title"] != "live" {
		t.Fatalf("unexpected realtime event: %+v", ev)
	}
}

func readSSE(t *testing.T, r *bufio.Reader) (event, id, data string) {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return event, id, data
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "id: "):
			id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestMetricsAndVersion(t *testing.T) {
	api := newTestAPI(t)

	// Test /metrics
	resp := api.request("GET", "/metrics", nil, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	if !strings.Contains(text, "docpocket_uptime_seconds") ||
		!strings.Contains(text, "docpocket_db_size_bytes") ||
		!strings.Contains(text, "docpocket_collections_total") {
		t.Fatalf("unexpected metrics body:\n%s", text)
	}

	// Test /api/version
	ver := api.json("GET", "/api/version", nil, "", http.StatusOK)
	if ver["version"] != "test" {
		t.Fatalf("unexpected version response: %v", ver)
	}

	// Test UI serving
	uiResp := api.request("GET", "/_/", nil, "", "")
	defer uiResp.Body.Close()
	if uiResp.StatusCode != http.StatusOK {
		t.Fatalf("UI status %d", uiResp.StatusCode)
	}
	uiBody, _ := io.ReadAll(uiResp.Body)
	if !strings.Contains(string(uiBody), "DocPocket") {
		t.Fatalf("UI body does not contain DocPocket: %s", string(uiBody))
	}
}

func TestAdminRuleTester(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()

	// Direct rule test with admin credentials
	res := api.json("POST", "/api/admin/rules/test", map[string]any{
		"rule":      "request.auth.id == 'u1'",
		"operation": "view",
		"auth":      map[string]any{"id": "u1"},
	}, api.token, http.StatusOK)
	if res["allowed"] != true {
		t.Fatalf("expected allowed: %v", res)
	}

	// Denied test
	res = api.json("POST", "/api/admin/rules/test", map[string]any{
		"rule":      "request.auth.id == 'u1'",
		"operation": "view",
		"auth":      map[string]any{"id": "u2"},
	}, api.token, http.StatusOK)
	if res["allowed"] != false {
		t.Fatalf("expected denied: %v", res)
	}
}

func TestAdminRecordsListAndBackup(t *testing.T) {
	api := newTestAPI(t)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))

	// Create a record
	api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "admin test post",
	}, "", http.StatusCreated)

	// Admin records listing
	list := api.json("GET", "/api/admin/collections/posts/records", nil, api.token, http.StatusOK)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// Admin backup download
	resp := api.request("GET", "/api/admin/backup", nil, api.token, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup status %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/octet-stream") {
		t.Fatalf("unexpected content type: %q", resp.Header.Get("Content-Type"))
	}
	backupData, err := io.ReadAll(resp.Body)
	if err != nil || len(backupData) == 0 {
		t.Fatalf("failed to read backup: len=%d, err=%v", len(backupData), err)
	}
}

func TestRecordHooks(t *testing.T) {
	var (
		beforeCreateCalled bool
		afterCreateCalled  bool
		beforeUpdateCalled bool
		afterUpdateCalled  bool
		beforeDeleteCalled bool
		afterDeleteCalled  bool
	)

	hooks := Hooks{
		OnRecordBeforeCreate: func(col string, doc collections.Document, id *auth.Identity) error {
			beforeCreateCalled = true
			if doc["block"] == true {
				return errors.New("blocked by hook")
			}
			return nil
		},
		OnRecordAfterCreate: func(col string, doc collections.Document, id *auth.Identity) {
			afterCreateCalled = true
		},
		OnRecordBeforeUpdate: func(col string, doc, old collections.Document, id *auth.Identity) error {
			beforeUpdateCalled = true
			return nil
		},
		OnRecordAfterUpdate: func(col string, doc, old collections.Document, id *auth.Identity) {
			afterUpdateCalled = true
		},
		OnRecordBeforeDelete: func(col string, doc collections.Document, id *auth.Identity) error {
			beforeDeleteCalled = true
			return nil
		},
		OnRecordAfterDelete: func(col string, doc collections.Document, id *auth.Identity) {
			afterDeleteCalled = true
		},
	}

	api := newTestAPIWithHooks(t, hooks)
	api.setupAdmin()
	api.createCollection(publicBaseCollection("posts"))

	// Blocked by hook
	api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "blocked", "block": true,
	}, "", http.StatusBadRequest)

	// Successful create
	created := api.json("POST", "/api/collections/posts/records", map[string]any{
		"title": "hook test",
	}, "", http.StatusCreated)
	if !beforeCreateCalled || !afterCreateCalled {
		t.Fatalf("create hooks not called: before=%v after=%v", beforeCreateCalled, afterCreateCalled)
	}

	id := created["id"].(string)

	// Update
	api.json("PATCH", "/api/collections/posts/records/"+id, map[string]any{
		"title": "updated title",
	}, "", http.StatusOK)
	if !beforeUpdateCalled || !afterUpdateCalled {
		t.Fatalf("update hooks not called: before=%v after=%v", beforeUpdateCalled, afterUpdateCalled)
	}

	// Delete
	api.json("DELETE", "/api/collections/posts/records/"+id, nil, "", http.StatusNoContent)
	if !beforeDeleteCalled || !afterDeleteCalled {
		t.Fatalf("delete hooks not called: before=%v after=%v", beforeDeleteCalled, afterDeleteCalled)
	}
}
