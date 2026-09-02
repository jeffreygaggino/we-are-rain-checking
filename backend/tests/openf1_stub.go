package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// OpenF1Stub stands in for the OpenF1 API at the HTTP boundary rather than at a Go interface, so a
// test that drives ingest also exercises the client's decoding, its 404 branch and its pacing. See
// plans/01-backend-v1.md, "No interface at the ingest seam".
//
// The wire shapes below are OpenF1's own, copied from a probe of the live API. That is the point:
// if the client's json tags drift from the upstream, these tests are what notices.
type OpenF1Stub struct {
	server *httptest.Server

	mu       sync.Mutex
	meetings map[int][]StubMeeting
	sessions map[int][]StubSession
	faults   map[string]stubFault
	delay    time.Duration
	requests []StubRequest
}

// StubMeeting is one row of `GET /meetings`, in the upstream's field names. Only the fields ingest
// reads are here; the live response carries flag images and circuit artwork too.
type StubMeeting struct {
	MeetingKey          int    `json:"meeting_key"`
	MeetingName         string `json:"meeting_name"`
	MeetingOfficialName string `json:"meeting_official_name"`
	Location            string `json:"location"`
	CountryName         string `json:"country_name"`
	CircuitKey          int    `json:"circuit_key"`
	CircuitShortName    string `json:"circuit_short_name"`
	DateStart           string `json:"date_start"`
	Year                int    `json:"year"`
	IsCancelled         bool   `json:"is_cancelled"`
}

// StubSession is one row of `GET /sessions`.
type StubSession struct {
	SessionKey       int    `json:"session_key"`
	MeetingKey       int    `json:"meeting_key"`
	SessionType      string `json:"session_type"`
	SessionName      string `json:"session_name"`
	Location         string `json:"location"`
	CountryName      string `json:"country_name"`
	CircuitKey       int    `json:"circuit_key"`
	CircuitShortName string `json:"circuit_short_name"`
	DateStart        string `json:"date_start"`
	DateEnd          string `json:"date_end"`
	Year             int    `json:"year"`
	IsCancelled      bool   `json:"is_cancelled"`
}

// StubRequest is one call the stub served. At is what the pacing assertion measures.
type StubRequest struct {
	Path string
	Year int
	At   time.Time
}

type stubFault struct {
	status int
	body   string
}

// NewOpenF1Stub starts a stub upstream that 404s every season until one is set — which is exactly
// what the live API does for a year it does not carry.
func NewOpenF1Stub(t *testing.T) *OpenF1Stub {
	t.Helper()

	s := &OpenF1Stub{
		meetings: map[int][]StubMeeting{},
		sessions: map[int][]StubSession{},
		faults:   map[string]stubFault{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

// BaseURL is what the client under test is pointed at.
func (s *OpenF1Stub) BaseURL() string { return s.server.URL }

// SetSeason gives the stub a year's worth of data. A year never set 404s as "No results found.".
func (s *OpenF1Stub) SetSeason(year int, meetings []StubMeeting, sessions []StubSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meetings[year] = meetings
	s.sessions[year] = sessions
}

// FailNext arms a one-shot fault on one path for one season. It is keyed on the year because "the
// run died partway" means a specific season died, not the first one asked for; consuming the fault
// on use is what lets the same stub then answer a re-run healthily.
func (s *OpenF1Stub) FailNext(path string, year, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[faultKey(path, year)] = stubFault{status: status, body: body}
}

func faultKey(path string, year int) string { return fmt.Sprintf("%s?year=%d", path, year) }

// Delay holds every response back, for asserting the client's own timeout rather than the upstream's.
func (s *OpenF1Stub) Delay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = d
}

// Requests returns what has been asked of the upstream, in order. A test asserts on this to show a
// season was skipped rather than merely re-written with the same rows.
func (s *OpenF1Stub) Requests() []StubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]StubRequest(nil), s.requests...)
}

// ResetRequests clears the log so a second run's requests can be asserted on their own.
func (s *OpenF1Stub) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

func (s *OpenF1Stub) serve(w http.ResponseWriter, r *http.Request) {
	year := 0
	if _, err := fmt.Sscanf(r.URL.Query().Get("year"), "%d", &year); err != nil {
		year = 0
	}

	s.mu.Lock()
	s.requests = append(s.requests, StubRequest{Path: r.URL.Path, Year: year, At: time.Now()})
	fault, faulted := s.faults[faultKey(r.URL.Path, year)]
	if faulted {
		delete(s.faults, faultKey(r.URL.Path, year))
	}
	delay := s.delay
	meetings, haveMeetings := s.meetings[year]
	sessions, haveSessions := s.sessions[year]
	s.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	w.Header().Set("Content-Type", "application/json")

	if faulted {
		w.WriteHeader(fault.status)
		_, _ = w.Write([]byte(fault.body))
		return
	}

	var (
		body any
		have bool
	)
	switch r.URL.Path {
	case "/meetings":
		body, have = meetings, haveMeetings
	case "/sessions":
		body, have = sessions, haveSessions
	default:
		have = false
	}

	// The live API's empty answer, byte for byte. It is a 404, which is why the client cannot read
	// status alone — see plans/01-backend-v1.md, "The 404 discrimination is defensive today".
	if !have {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"No results found."}`))
		return
	}

	_ = json.NewEncoder(w).Encode(body)
}
