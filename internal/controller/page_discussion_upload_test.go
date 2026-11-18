package controller

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nexmap.net/internal/config"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
	"nexmap.net/internal/testutil"
	"nexmap.net/web/templates/patterns"
)

const testCSRFToken = "csrf-token-001"

// uploadRequest builds a real multipart POST for the attachment create handler.
func uploadRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	r, _ := uploadRequestWithBody(t, filename, content)
	return r
}

// uploadRequestWithBody also returns the underlying reader, so a test can
// assert how much of the request was actually consumed. Checking r.Body is
// useless once it has been replaced by an exhausted MaxBytesReader.
func uploadRequestWithBody(t *testing.T, filename string, content []byte) (*http.Request, *bytes.Reader) {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// The real form posts the CSRF token alongside the file.
	if err := mw.WriteField("_csrf", testCSRFToken); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	raw := bytes.NewReader(body.Bytes())
	r := httptest.NewRequest(http.MethodPost, "/products/product-001/components/component-001/attachments", raw)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.SetPathValue("id", "product-001")
	r.SetPathValue("componentId", "component-001")

	r = r.WithContext(middleware.WithUser(r.Context(), testutil.MakeUser(), "session", testCSRFToken))
	return r, raw
}

// newUploadController wires the real service layer over mock stores, with an
// upload policy that only accepts logs.
func newUploadController(t *testing.T, maxBytes int64) *PageProductController {
	t.Helper()

	blobs, err := storage.NewFS(t.TempDir(), maxBytes)
	if err != nil {
		t.Fatalf("NewFS: %v", err)
	}

	st := &store.Store{
		ProductRole: &testutil.MockProductRoleStore{
			GetByUserAndProductFunc: func(_ context.Context, _, _ string) (*model.ProductRole, error) {
				return testutil.MakeProductRole(), nil
			},
		},
		Product: &testutil.MockProductStore{
			GetByIDFunc: func(_ context.Context, _ string) (*model.Product, error) {
				return testutil.MakeProduct(), nil
			},
		},
		User: &testutil.MockUserStore{
			ListFunc: func(_ context.Context, _ store.UserFilter) ([]*model.User, error) {
				return []*model.User{testutil.MakeUser()}, nil
			},
		},
		// The upload targets component-001, which must belong to the product
		// for the ownership check to let it through.
		Component: &testutil.MockComponentStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.Component, error) {
				return testutil.MakeComponent(func(c *model.Component) { c.ID = id }), nil
			},
		},
		Comment: &testutil.MockCommentStore{},
		Attachment: &testutil.MockAttachmentStore{
			CreateFunc: func(_ context.Context, _ *model.Attachment) error { return nil },
			// The tab label's count is refreshed after an upload (UC-88).
			ListByOwnerFunc: func(_ context.Context, _, _, _ string) ([]*model.Attachment, error) { return nil, nil },
		},
		Audit: &testutil.MockAuditStore{
			CreateFunc: func(_ context.Context, _ *model.AuditEntry) error { return nil },
		},
	}

	cfg := &config.Config{
		MaxUploadBytes: maxBytes,
		AllowedUploads: "logs",
	}
	svc := service.New(st, &testutil.MockAuthProvider{}, cfg, blobs)

	return NewPageProduct(
		svc.Product, svc.Dashboard, svc.Setting, svc.User, svc.Invitation,
		svc.Milestone, svc.Component, svc.Testing, svc.Backlog, svc.Category,
		svc.Constraint, svc.Discussion, svc.Audit, maxBytes,
	)
}

// A rejected upload must reach the browser as something htmx will act on.
// htmx discards non-2xx responses entirely, so returning 415 here means the
// user sees nothing at all — which is exactly the bug this guards against.
func TestUploadRejectionIsRenderedForHTMX(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  []byte
		wantIn   string
	}{
		{
			name:     "disallowed extension",
			filename: "payload.zip",
			content:  []byte{'P', 'K', 0x03, 0x04},
			wantIn:   "not allowed",
		},
		{
			name:     "content contradicts the extension",
			filename: "notes.log",
			content:  []byte{'P', 'K', 0x03, 0x04},
			wantIn:   "not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := newUploadController(t, 1<<20)
			w := httptest.NewRecorder()

			ctrl.ComponentAttachmentCreate(w, uploadRequest(t, tt.filename, tt.content))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 — htmx ignores non-2xx and the user sees nothing", w.Code)
			}
			if w.Header().Get(discussionErrorHeader) == "" {
				t.Fatal("missing the rejection header; the form would reset and lose the user's input")
			}

			body := w.Body.String()
			for _, want := range []string{
				`id="` + patterns.AttachmentsErrorRegion + `"`,
				`hx-swap-oob="innerHTML"`,
				tt.wantIn,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("body lacks %q: %s", want, body)
				}
			}
			// The primary target appends with beforeend, so any markup outside
			// the out-of-band div would be injected into the list.
			if strings.Contains(body, "<li") {
				t.Fatalf("error response would append a row to the list: %s", body)
			}
		})
	}
}

func TestSuccessfulUploadClearsTheErrorRegion(t *testing.T) {
	ctrl := newUploadController(t, 1<<20)
	w := httptest.NewRecorder()

	ctrl.ComponentAttachmentCreate(w, uploadRequest(t, "app.log", []byte("2026-01-01 INFO ok\n")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get(discussionErrorHeader); got != "" {
		t.Fatalf("success set the rejection header (%q); the form would not reset", got)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<li") {
		t.Fatalf("success did not render the new row: %s", body)
	}
	if !strings.Contains(body, `id="`+patterns.AttachmentsErrorRegion+`" hx-swap-oob="innerHTML"></div>`) {
		t.Fatalf("success did not clear the error region: %s", body)
	}
	if !strings.Contains(body, `id="attachments-empty" hx-swap-oob="innerHTML"></div>`) {
		t.Fatalf("success did not clear the empty state: %s", body)
	}
}

// The upload date must be part of the rendered row (regression: it was absent).
func TestUploadedRowShowsTheDate(t *testing.T) {
	ctrl := newUploadController(t, 1<<20)
	w := httptest.NewRecorder()

	ctrl.ComponentAttachmentCreate(w, uploadRequest(t, "app.log", []byte("hello")))

	body := w.Body.String()
	if !strings.Contains(body, "text/plain") && !strings.Contains(body, "5 B") {
		t.Fatalf("row is missing its metadata line: %s", body)
	}
	// formatTimestamp renders "2 Jan 2006, 15:04"; the year is a stable marker.
	if !strings.Contains(body, "202") {
		t.Fatalf("row is missing the upload date: %s", body)
	}
}

// uploadChain mirrors the router's real wrapping: body limit, then CSRF, then
// the handler. The CSRF middleware reads the form, so an oversize body must be
// reported before it runs or the user gets an opaque 403.
func uploadChain(ctrl *PageProductController) http.Handler {
	return ctrl.LimitUploadBody(middleware.CSRF(http.HandlerFunc(ctrl.ComponentAttachmentCreate)))
}

func TestOversizeUploadIsReportedNotSwallowedByCSRF(t *testing.T) {
	const maxBytes = 4 << 10

	// Well past maxBytes + the 1 MiB envelope, so the body limit trips during
	// parsing rather than in the storage layer.
	huge := bytes.Repeat([]byte("x"), int(maxBytes)+(2<<20))

	ctrl := newUploadController(t, maxBytes)
	w := httptest.NewRecorder()
	uploadChain(ctrl).ServeHTTP(w, uploadRequest(t, "huge.log", huge))

	if w.Code == http.StatusForbidden {
		t.Fatal("oversize upload was rejected by CSRF; the user gets no message at all")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — htmx ignores non-2xx", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "too large") {
		t.Fatalf("body does not explain the size limit: %s", body)
	}
	if !strings.Contains(body, `id="`+patterns.AttachmentsErrorRegion+`"`) {
		t.Fatalf("body does not target the error region: %s", body)
	}
}

// A file over the limit but under the envelope headroom is caught by storage
// instead. Both paths must produce the same message.
func TestOversizeUploadJustOverTheLimitIsReported(t *testing.T) {
	const maxBytes = 4 << 10

	justOver := bytes.Repeat([]byte("x"), int(maxBytes)+512)

	ctrl := newUploadController(t, maxBytes)
	w := httptest.NewRecorder()
	uploadChain(ctrl).ServeHTTP(w, uploadRequest(t, "big.log", justOver))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too large") {
		t.Fatalf("body does not explain the size limit: %s", w.Body.String())
	}
}

// The originally reported combination: oversize *and* a disallowed type.
func TestOversizeAndDisallowedTypeIsReported(t *testing.T) {
	const maxBytes = 4 << 10

	huge := append([]byte{'P', 'K', 0x03, 0x04}, bytes.Repeat([]byte("x"), int(maxBytes)+(2<<20))...)

	ctrl := newUploadController(t, maxBytes)
	w := httptest.NewRecorder()
	uploadChain(ctrl).ServeHTTP(w, uploadRequest(t, "huge.zip", huge))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="`+patterns.AttachmentsErrorRegion+`"`) {
		t.Fatalf("no error rendered for an oversize, disallowed file: %s", body)
	}
}

// A valid upload must still pass through the whole chain, CSRF included.
func TestValidUploadSurvivesTheChain(t *testing.T) {
	ctrl := newUploadController(t, 1<<20)
	w := httptest.NewRecorder()
	uploadChain(ctrl).ServeHTTP(w, uploadRequest(t, "app.log", []byte("all good\n")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get(discussionErrorHeader); got != "" {
		t.Fatalf("valid upload was flagged as an error (%q)", got)
	}
	if !strings.Contains(w.Body.String(), "<li") {
		t.Fatalf("valid upload did not render a row: %s", w.Body.String())
	}
}

// The reported failure: a file far larger than the cap. Go stops reading and
// marks the connection for close, so without draining the client is reset
// before it can read the reply.
func TestFarOversizeUploadStillGetsAResponse(t *testing.T) {
	const maxBytes = 10 << 20 // the production default

	ctrl := newUploadController(t, maxBytes)
	w := httptest.NewRecorder()
	req, raw := uploadRequestWithBody(t, "huge.log", bytes.Repeat([]byte("x"), 32<<20))

	uploadChain(ctrl).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too large") {
		t.Fatalf("no size message: %s", w.Body.String())
	}

	// The request must be fully consumed. If bytes are left, Go closes the
	// connection mid-upload and the client is reset before it can read the
	// response — the exact failure this test exists for.
	if left := raw.Len(); left > 0 {
		t.Fatalf("%d bytes of the request body were left unread; the client would be reset", left)
	}
}

// The upload form must carry everything the client-side guard needs; without
// these the browser sends the file and hits the reset described above.
func TestUploadFormCarriesTheClientSideGuard(t *testing.T) {
	var sb strings.Builder
	err := patterns.AttachmentList(nil, "/upload", "tok", true, 10<<20).
		Render(context.Background(), &sb)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := sb.String()

	for _, want := range []string{
		`hx-on::before-request="nexmapGuardUploadSize(event, this)"`,
		`data-max-bytes="10485760"`,
		`data-size-error-id="attachments-size-error"`,
		`data-server-error-id="attachments-error"`,
		`id="attachments-size-error"`,
		"function nexmapGuardUploadSize",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("upload form is missing %q", want)
		}
	}

	// The pre-rendered banner must already state the limit, since the guard
	// only unhides it and never builds markup.
	if !strings.Contains(out, "10.0 MB") {
		t.Fatalf("client-side banner does not state the limit: %s", out)
	}
}
