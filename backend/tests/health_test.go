package tests_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/db"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// The health route reports database connectivity, not process liveness. A handler that returned 200
// without touching the database would pass a test that only asserted the status code, so this one
// asserts the payload names what was checked.
func TestHealthReportsDatabaseConnectivityWhenPostgresIsReachable(t *testing.T) {
	h := tests.RequireHarness(t)

	rec := h.GET(t, "/api/v1/health")

	var report models.HealthReport
	tests.DecodeSuccess(t, rec, http.StatusOK, &report)
	if report.Database != models.HealthOK {
		t.Errorf("database = %q, want %q", report.Database, models.HealthOK)
	}
}

// The other half of the same claim: with the database gone, the route fails. Together these two are
// what makes the route worth having — one alone is satisfied by a handler that always answers.
func TestHealthFailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	unreachable := &tests.Harness{Router: tests.NewRouter(unreachablePool(t)), DB: nil}

	rec := unreachable.GET(t, "/api/v1/health")

	envelope := tests.DecodeError(t, rec, http.StatusServiceUnavailable)
	// The message names the dependency that failed. A bare "internal error" here sends the next
	// person debugging into the service rather than at its database.
	if !strings.Contains(strings.ToLower(envelope.Message), "database") {
		t.Errorf("message = %q, want it to name the database", envelope.Message)
	}
}

// #2: no route returns a bare object. Gin's own 404 is plain text, so this fails unless the router
// hands unmatched paths to the shared envelope.
func TestAnUnknownRouteAnswersThroughTheErrorEnvelope(t *testing.T) {
	h := tests.RequireHarness(t)

	rec := h.GET(t, "/api/v1/no-such-route")

	tests.DecodeError(t, rec, http.StatusNotFound)
}

// The generated spec is served by the same router as the routes it documents, so a route added
// without regenerating docs is visible here rather than at whatever a reader assumes.
func TestGeneratedDocsAreServedAndDescribeTheHealthRoute(t *testing.T) {
	h := tests.RequireHarness(t)

	rec := h.GET(t, "/docs/doc.json")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"/health"`) {
		t.Errorf("generated spec does not describe /health — run `make docs`")
	}
}

// ADR-0001 and #2, asserted rather than assumed: there is no auth layer to send credentials to and
// no browser client to allow, so the health route answers an unauthenticated request and no response
// carries a CORS header. Adding either becomes a decision someone has to make against this test.
func TestNoAuthOrCORSLayerStandsInFrontOfTheRoutes(t *testing.T) {
	h := tests.RequireHarness(t)

	rec := h.GET(t, "/api/v1/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("an unauthenticated request got %d, want 200", rec.Code)
	}
	for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "WWW-Authenticate"} {
		if got := rec.Header().Get(header); got != "" {
			t.Errorf("response carries %s: %q", header, got)
		}
	}
}

// unreachablePool returns a pool that parses but cannot connect: sqlx.Open is lazy, so the failure
// lands where the health route pings rather than here.
func unreachablePool(t *testing.T) *sqlx.DB {
	t.Helper()

	_, cfg := tests.RequireDB(t)

	gone := *cfg
	// Port 1 is closed, so the dial is refused immediately rather than after a connect timeout.
	gone.DBPort = "1"

	conn, err := sqlx.Open("postgres", db.DSN(&gone))
	if err != nil {
		t.Fatalf("opening a deliberately unreachable pool: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
