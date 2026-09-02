package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/app"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

// Harness is seam 1: the real router, wired by the same composition root the binary uses, over a
// real Postgres pool.
//
// Later tickets add endpoints by wiring them into app.New, which is what keeps this file from
// growing a branch per route.
type Harness struct {
	Router *gin.Engine
	DB     *sqlx.DB
}

// RequireHarness stands the whole stack up against the package's database, wound back to a clean
// slate.
func RequireHarness(t *testing.T) *Harness {
	t.Helper()

	conn, _ := RequireDB(t)
	return &Harness{Router: NewRouter(conn), DB: conn}
}

// RouterOnly is the same router with no database behind it, for the tests whose subject is routing
// itself: the envelope on an unknown path, the generated spec, and the absence of an auth layer in
// front of the routes. None of them opens a connection, so none of them waits for a schema.
func RouterOnly(t *testing.T) *Harness {
	t.Helper()

	return &Harness{Router: NewRouter(nil), DB: nil}
}

// NewRouter builds the router over an arbitrary pool, so a test can point the service at a database
// that is not there and assert what it does about it.
func NewRouter(conn *sqlx.DB) *gin.Engine {
	// Quiets gin, in the two places it is noisy. TestMode drops the debug route table; the discarded
	// writer drops the per-request log lines, which come from gin.Default's logger and are not
	// affected by the mode at all. The logger captures the writer when the router is built, so both
	// lines must precede app.New. Errors keep their own writer, so a panic still reaches the run.
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard
	return app.New(conn)
}

// GET drives the router through net/http/httptest rather than a listening socket: same handler
// chain, same middleware, no port to collide on when tests run in parallel.
func (h *Harness) GET(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.Router.ServeHTTP(rec, req)
	return rec
}

// DecodeSuccess asserts the response is a success envelope with the expected status and unmarshals
// its data into `data` when one is passed.
func DecodeSuccess(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, data any) models.HttpResponse {
	t.Helper()

	var envelope models.HttpResponse
	decodeBody(t, rec, wantStatus, &envelope)
	if !envelope.Success {
		t.Fatalf("success = false on a %d; body = %s", rec.Code, rec.Body.String())
	}
	if envelope.Code != wantStatus {
		t.Errorf("envelope code = %d, want %d", envelope.Code, wantStatus)
	}
	if data == nil {
		return envelope
	}

	// Re-marshal rather than type-asserting: Data decodes as map[string]any, and the caller wants
	// the typed shape the handler actually returned.
	raw, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatalf("re-marshalling data: %v", err)
	}
	if err := json.Unmarshal(raw, data); err != nil {
		t.Fatalf("decoding data into %T: %v; data = %s", data, err, raw)
	}
	return envelope
}

// DecodeError asserts the response is an error envelope with the expected status.
func DecodeError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) models.ErrorResponse {
	t.Helper()

	var envelope models.ErrorResponse
	decodeBody(t, rec, wantStatus, &envelope)
	if envelope.Success {
		t.Errorf("success = true on a %d; body = %s", rec.Code, rec.Body.String())
	}
	if envelope.Code != wantStatus {
		t.Errorf("envelope code = %d, want %d", envelope.Code, wantStatus)
	}
	return envelope
}

// decodeBody is the half the two envelopes share: the status has to be right, and the body has to be
// the JSON shape claimed. A body that is not an envelope at all fails here, naming what was returned
// instead — which is what a route answering outside the envelope looks like.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, envelope any) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, wantStatus, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), envelope); err != nil {
		t.Fatalf("decoding %T: %v; body = %s", envelope, err, rec.Body.String())
	}
}
