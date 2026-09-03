package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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
	weather  map[int][]StubWeatherSample
	results  map[int][]StubSessionResult
	entrants map[int][]StubDriver
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

// StubWeatherSample is one row of `GET /weather`, in the upstream's field names.
//
// Rainfall is an int because that is what the wire carries. Staging it as a bool would move the
// mapping into the fixture and stop the test proving the client does it.
type StubWeatherSample struct {
	Date             string  `json:"date"`
	SessionKey       int     `json:"session_key"`
	MeetingKey       int     `json:"meeting_key"`
	WindDirection    int     `json:"wind_direction"`
	AirTemperature   float64 `json:"air_temperature"`
	Humidity         float64 `json:"humidity"`
	Pressure         float64 `json:"pressure"`
	Rainfall         int     `json:"rainfall"`
	WindSpeed        float64 `json:"wind_speed"`
	TrackTemperature float64 `json:"track_temperature"`
}

// StubSessionResult is one row of `GET /session_result`, in the upstream's field names.
//
// Every field the client reads is a pointer, because every one of them arrives null somewhere in the
// live data or is strict in the client — and a fixture that cannot omit a field cannot stage the
// row that proves the strictness.
//
// GapToLeader is raw JSON: upstream sends `0`, `11.987`, `"+1 LAP"` and `null` in the one field, and
// this repo stores none of them. Carrying it here as it arrives is what shows the client's decode is
// unbothered by the union rather than merely untested against it.
type StubSessionResult struct {
	Position     *int            `json:"position"`
	DriverNumber int             `json:"driver_number"`
	NumberOfLaps *int            `json:"number_of_laps"`
	Points       *float64        `json:"points"`
	DNF          *bool           `json:"dnf"`
	DNS          *bool           `json:"dns"`
	DSQ          *bool           `json:"dsq"`
	Duration     *float64        `json:"duration"`
	GapToLeader  json.RawMessage `json:"gap_to_leader,omitempty"`
	MeetingKey   int             `json:"meeting_key"`
	SessionKey   int             `json:"session_key"`
}

// StubDriver is one row of `GET /drivers` — one Session's entry list, which is the only thing that
// says which person carried a Racing Number that weekend (ADR-0003). The live row carries a headshot
// URL, a team colour and both name halves besides.
type StubDriver struct {
	MeetingKey    int    `json:"meeting_key"`
	SessionKey    int    `json:"session_key"`
	DriverNumber  int    `json:"driver_number"`
	BroadcastName string `json:"broadcast_name"`
	FullName      string `json:"full_name"`
	NameAcronym   string `json:"name_acronym"`
	TeamName      string `json:"team_name"`
}

// StubRequest is one call the stub served. At is what the pacing assertion measures.
//
// Year and SessionKey are both here because the two shapes of request this stub serves select
// differently — a season by `year`, a Session's weather by `session_key` — and whichever the path
// does not use stays zero.
type StubRequest struct {
	Path       string
	Year       int
	SessionKey int
	At         time.Time
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
		weather:  map[int][]StubWeatherSample{},
		results:  map[int][]StubSessionResult{},
		entrants: map[int][]StubDriver{},
		faults:   map[string]stubFault{},
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

// BaseURL is what the client under test is pointed at.
func (s *OpenF1Stub) BaseURL() string { return s.server.URL }

// SetSeason gives the stub a year's worth of data. A year never set 404s as "No results found.".
//
// The fixtures are copied, not retained: callers amend their own slice between runs to stage an
// upstream correction, and that write must not reach what the stub is already serving — serve
// encodes outside the lock, so with a request in flight it is a race. The copy is shallow, which is
// a complete one only while the fixture structs stay free of slices, maps and pointers.
func (s *OpenF1Stub) SetSeason(year int, meetings []StubMeeting, sessions []StubSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meetings[year] = slices.Clone(meetings)
	s.sessions[year] = slices.Clone(sessions)
}

// SetWeather gives the stub one Session's Weather Samples. A Session never set — and one set back to
// nil — 404s as "No results found.", which is what the live API does for a Session it holds no
// weather for, a cancelled one included. Copied for the same reason SetSeason copies.
//
// nil unsets rather than staging an empty array, because "the upstream has nothing for this Session"
// arrives as a 404 and never as `200 []`. Staged as a present-but-empty fixture, a test claiming to
// exercise the 404 discrimination would pass without ever reaching it.
func (s *OpenF1Stub) SetWeather(sessionKey int, samples []StubWeatherSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if samples == nil {
		delete(s.weather, sessionKey)
		return
	}
	s.weather[sessionKey] = slices.Clone(samples)
}

// SetSessionResults gives the stub one Session's classification. A Session never set — or set back
// to nil — 404s as "No results found.", what the live API does for a Race still to run or a
// cancelled one, probed against `session_result?session_key=9086` on 2026-09-03. Copied for the
// reason SetSeason copies, and nil unsets for the reason SetWeather's does.
func (s *OpenF1Stub) SetSessionResults(sessionKey int, results []StubSessionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if results == nil {
		delete(s.results, sessionKey)
		return
	}
	s.results[sessionKey] = slices.Clone(results)
}

// SetSessionDrivers gives the stub one Session's entry list. Set separately from the results so a
// test can stage the two disagreeing — a number the entry list does not carry, or one carried twice
// — which is where ADR-0003's resolution either holds or silently merges two people.
func (s *OpenF1Stub) SetSessionDrivers(sessionKey int, drivers []StubDriver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if drivers == nil {
		delete(s.entrants, sessionKey)
		return
	}
	s.entrants[sessionKey] = slices.Clone(drivers)
}

// FailNext arms a one-shot fault on one path for one season. It is keyed on the year because "the
// run died partway" means a specific season died, not the first one asked for; consuming the fault
// on use is what lets the same stub then answer a re-run healthily.
func (s *OpenF1Stub) FailNext(path string, year, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[faultKey(path, year, 0)] = stubFault{status: status, body: body}
}

// FailNextForSession is FailNext for a path selected by Session rather than by season, which is what
// makes "the run died on this Session's weather" stageable.
func (s *OpenF1Stub) FailNextForSession(path string, sessionKey, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[faultKey(path, 0, sessionKey)] = stubFault{status: status, body: body}
}

func faultKey(path string, year, sessionKey int) string {
	return fmt.Sprintf("%s?year=%d&session_key=%d", path, year, sessionKey)
}

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
	query := r.URL.Query()
	year := intParam(query.Get("year"))
	sessionKey := intParam(query.Get("session_key"))

	s.mu.Lock()
	s.requests = append(s.requests, StubRequest{
		Path:       r.URL.Path,
		Year:       year,
		SessionKey: sessionKey,
		At:         time.Now(),
	})
	key := faultKey(r.URL.Path, year, sessionKey)
	fault, faulted := s.faults[key]
	if faulted {
		delete(s.faults, key)
	}
	delay := s.delay
	meetings, haveMeetings := s.meetings[year]
	sessions, haveSessions := s.sessions[year]
	weather, haveWeather := s.weather[sessionKey]
	results, haveResults := s.results[sessionKey]
	entrants, haveEntrants := s.entrants[sessionKey]
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
	case "/weather":
		body, have = weather, haveWeather
	case "/session_result":
		body, have = results, haveResults
	case "/drivers":
		body, have = entrants, haveEntrants
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

// intParam reads a numeric query value, treating absent and unparseable alike as 0 — which selects
// no fixture and so 404s, exactly as the live API does for a season or Session it does not carry.
func intParam(value string) int {
	n := 0
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0
	}
	return n
}
