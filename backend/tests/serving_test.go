package tests_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/app"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// #12: the deployed service must not inherit gin's development defaults. Everything here asserts a
// value rather than reading a log, because a log line is only caught by someone who happened to look.
const (
	// httptest.NewRequest's fixed peer address, and a documentation address that is never a peer.
	peerIP      = "192.0.2.1"
	forwardedIP = "203.0.113.9"
)

// The claim the ticket is actually about. Not that some line calls SetMode — that nobody has to
// choose a mode for the deployed service to stop announcing debug and printing its route table.
func TestTheServingModeIsReleaseWhenNothingChoosesOne(t *testing.T) {
	restoreConfig(t)
	t.Setenv("GIN_MODE", "")

	if got := tests.RequireConfig(t).GinMode; got != gin.ReleaseMode {
		t.Errorf("GinMode = %q with GIN_MODE unset, want %q", got, gin.ReleaseMode)
	}
}

// The other half: a developer running this locally still gets the debug output. .env.example sets it
// and both `make run` and `make ingest` source that file; the local compose file sets it again for
// `make local`, which does not read .env at all.
func TestTheServingModeHonoursAnExplicitDebugChoice(t *testing.T) {
	restoreConfig(t)
	t.Setenv("GIN_MODE", gin.DebugMode)

	if got := tests.RequireConfig(t).GinMode; got != gin.DebugMode {
		t.Errorf("GinMode = %q with GIN_MODE=debug, want %q", got, gin.DebugMode)
	}
}

// gin.Default() trusts every proxy, so ClientIP() believes whatever X-Forwarded-For a caller sends.
// This service is published through a proxy by design, which makes that reachable rather than
// theoretical.
func TestASpoofedForwardedAddressIsIgnoredWhenNoProxyIsTrusted(t *testing.T) {
	router := tests.NewRouter(t, nil)

	got := clientIPBehind(t, router)

	if got == forwardedIP {
		t.Fatalf("client address = %q — the router believed an untrusted X-Forwarded-For", got)
	}
	if got != peerIP {
		t.Errorf("client address = %q, want the peer address %q", got, peerIP)
	}
}

// The paired positive case. Without it, an app.New that parsed TRUSTED_PROXIES and then dropped it
// would pass the test above — the list has to reach gin, not merely be read from the environment.
func TestAConfiguredProxyIsBelievedAboutTheClientAddress(t *testing.T) {
	router := tests.NewRouterTrusting(t, nil, []string{peerIP})

	got := clientIPBehind(t, router)

	if got != forwardedIP {
		t.Errorf("client address = %q, want the forwarded address %q from the trusted peer", got, forwardedIP)
	}
}

// The loader does not parse TRUSTED_PROXIES — gin does, and app.New hands its complaint back so the
// boot fails on a typo rather than serving with a policy nobody can see. Asserted here because that
// reasoning is only sound while the failure is reachable.
func TestAMalformedTrustedProxyIsRejectedRatherThanIgnored(t *testing.T) {
	_, err := app.New(nil, []string{"not-a-cidr"})

	if err == nil {
		t.Fatal("app.New accepted a proxy list gin cannot parse")
	}
	if !strings.Contains(err.Error(), "not-a-cidr") {
		t.Errorf("error = %q, want it to name the offending value", err)
	}
}

// #12 again, from the test side: the harness sets its own mode, so no test inherits whatever
// GIN_MODE the machine running it happens to carry.
func TestTheHarnessPicksItsOwnServingMode(t *testing.T) {
	// Wound to a mode the harness has to overwrite. Asserting against what an earlier test left
	// behind would pass on its own, since that leftover is already TestMode.
	gin.SetMode(gin.DebugMode)
	t.Cleanup(func() { gin.SetMode(gin.TestMode) })

	tests.NewRouter(t, nil)

	if gin.Mode() != gin.TestMode {
		t.Errorf("mode after NewRouter = %q, want %q", gin.Mode(), gin.TestMode)
	}
}

// clientIPBehind asks the engine what it believes the client address is, for a request arriving from
// the peer with a forwarded address attached.
//
// The route is registered here because no handler exposes ClientIP() yet — which is precisely why
// this ticket is cheap to land now and awkward once one does.
func clientIPBehind(t *testing.T, router *gin.Engine) string {
	t.Helper()

	router.GET("/probe/client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/probe/client-ip", nil)
	req.Header.Set("X-Forwarded-For", forwardedIP)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("probe route status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// restoreConfig reloads the process-global config once the test's environment is back.
//
// Registered before t.Setenv on purpose: cleanups run last-in-first-out, so this one has to be the
// outer of the two or it reloads from the doctored environment it exists to undo.
//
// Through RequireConfig rather than config.LoadConfig, here and above: LoadConfig fatals on a
// missing DB_HOST, which would take the whole test binary down with it and leave every later test in
// the package unreported. RequireConfig fails the test instead, naming `make test`.
func restoreConfig(t *testing.T) {
	t.Helper()

	t.Cleanup(func() { tests.RequireConfig(t) })
}
