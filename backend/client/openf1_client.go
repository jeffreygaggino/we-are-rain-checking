// Package client holds this service's outbound clients, one file per upstream.
//
// Each upstream's response shape stays in its own file. Nothing here decodes into models/ — a
// third party's field names are theirs, and the mapping to this repo's vocabulary is the whole job
// of the exported types below.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

// UpstreamOpenF1 names this dependency in an error, so a failure points at the service that failed
// rather than at this one.
const UpstreamOpenF1 = "openf1"

// noResultsDetail is OpenF1's empty answer. It arrives as a 404, which is also what a genuine fault
// looks like, so the body is the only thing that separates them. See plans/01-backend-v1.md.
const noResultsDetail = "No results found."

// errorBodyLimit caps what an error message quotes back. An upstream serving an HTML error page
// should not put a page into a log line.
const errorBodyLimit = 2 << 10

// OpenF1 emits an explicit numeric offset ("+00:00") rather than RFC3339's "Z" — but parse with
// RFC3339 anyway, which accepts both and a fractional second besides. Pinning the layout to today's
// exact spelling would turn a cosmetic upstream change into a total ingest outage, and the format
// the upstream actually sends is asserted by the fixtures rather than by the parser.

// OpenF1Client reads the OpenF1 API. One client per process: the pacer it carries is the only thing
// keeping ingest under the documented rate limit, and a second client would double the rate.
type OpenF1Client struct {
	baseURL string
	http    *http.Client
	pacer   *pacer
}

// NewOpenF1Client builds a client bound by an explicit timeout. Small JSON exchanges, so one
// whole-request timeout is the right bound; minInterval of 0 disables pacing, which is what tests
// that are not about pacing pass.
func NewOpenF1Client(baseURL string, timeout, minInterval time.Duration) *OpenF1Client {
	return &OpenF1Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
		pacer:   &pacer{min: minInterval},
	}
}

// Meeting is one Grand Prix weekend as OpenF1 reports it, mapped onto this repo's names. CircuitKey
// is the upstream's circuit identifier, not a Circuit: resolving it to a seeded id is the caller's
// job, because only the caller has the database.
type Meeting struct {
	MeetingKey   int
	Year         int
	Name         string
	OfficialName string
	CircuitKey   int
	CountryName  string
	Location     string
	DateStart    time.Time
}

// Session is one timed running of cars, as above.
type Session struct {
	SessionKey  int
	MeetingKey  int
	CircuitKey  int
	SessionType string
	SessionName string
	Year        int
	DateStart   time.Time
	DateEnd     time.Time
	IsCancelled bool
}

// WeatherSample is one timestamped observation at the Circuit during a Session.
//
// Rainfall becomes a bool here, at the boundary, so nothing downstream can read a magnitude into a
// flag that has none (CONTEXT.md, Rainfall). Everything else is a pointer: an observation the
// upstream did not take is absent rather than zero, and wind speed in particular records a real
// 0 m/s.
type WeatherSample struct {
	SessionKey       int
	ObservedAt       time.Time
	Rainfall         bool
	AirTemperature   *float64
	TrackTemperature *float64
	Humidity         *float64
	Pressure         *float64
	WindSpeed        *float64
	WindDirection    *int
}

// SessionResult is one Driver's classification in one Session, as the upstream reports it.
//
// RacingNumber is the upstream's `driver_number` and is not an identity — resolving it to a seeded
// Driver is the caller's job, per Session, because only the caller has the database (ADR-0003).
//
// Position and NumberOfLaps are pointers because a Retirement genuinely has neither. The three flags
// are not: they are what #10 counts, so a missing one fails at the boundary rather than reading as a
// finish. See plans/04-session-results.md.
type SessionResult struct {
	SessionKey   int
	RacingNumber int
	Position     *int
	Points       float64
	NumberOfLaps *int
	DNF          bool
	DNS          bool
	DSQ          bool
}

// SessionDriver is one entry of a Session's entry list: the mapping from a Racing Number to the
// upstream's display name, for that Session and no other.
type SessionDriver struct {
	SessionKey   int
	RacingNumber int
	FullName     string
}

// meetingRow, sessionRow and weatherRow are the wire. Timestamps arrive as strings rather than
// time.Time so a missing or malformed one is an error naming the field, instead of a zero time
// flowing into a NOT NULL column.
type meetingRow struct {
	MeetingKey   int     `json:"meeting_key"`
	Year         int     `json:"year"`
	Name         string  `json:"meeting_name"`
	OfficialName string  `json:"meeting_official_name"`
	CircuitKey   int     `json:"circuit_key"`
	CountryName  string  `json:"country_name"`
	Location     string  `json:"location"`
	DateStart    *string `json:"date_start"`
}

type sessionRow struct {
	SessionKey  int     `json:"session_key"`
	MeetingKey  int     `json:"meeting_key"`
	CircuitKey  int     `json:"circuit_key"`
	SessionType string  `json:"session_type"`
	SessionName string  `json:"session_name"`
	Year        int     `json:"year"`
	DateStart   *string `json:"date_start"`
	DateEnd     *string `json:"date_end"`
	IsCancelled bool    `json:"is_cancelled"`
}

// Rainfall is a *int for the same reason the timestamps are *string: decoding an absent one into 0
// would store "it was dry" over an observation the upstream never took.
type weatherRow struct {
	SessionKey       int      `json:"session_key"`
	Date             *string  `json:"date"`
	Rainfall         *int     `json:"rainfall"`
	AirTemperature   *float64 `json:"air_temperature"`
	TrackTemperature *float64 `json:"track_temperature"`
	Humidity         *float64 `json:"humidity"`
	Pressure         *float64 `json:"pressure"`
	WindSpeed        *float64 `json:"wind_speed"`
	WindDirection    *int     `json:"wind_direction"`
}

// sessionResultRow is the wire. Every measured field is a pointer, so "the upstream sent null" and
// "the upstream sent 0" stay different facts — points 0 is most of the grid, and dnf false is a
// finish.
//
// `gap_to_leader` and `duration` are absent deliberately: the first is a union of 0, 11.987,
// "+1 LAP" and null in one field, neither is read by any endpoint, and Go ignores what it has no
// field for. See plans/01-backend-v1.md.
type sessionResultRow struct {
	SessionKey   int      `json:"session_key"`
	DriverNumber *int     `json:"driver_number"`
	Position     *int     `json:"position"`
	Points       *float64 `json:"points"`
	NumberOfLaps *int     `json:"number_of_laps"`
	DNF          *bool    `json:"dnf"`
	DNS          *bool    `json:"dns"`
	DSQ          *bool    `json:"dsq"`
}

type sessionDriverRow struct {
	SessionKey   int    `json:"session_key"`
	DriverNumber *int   `json:"driver_number"`
	FullName     string `json:"full_name"`
}

// Meetings returns every Meeting of a season. A season the upstream does not carry is an empty
// slice and no error — that is an answer, not a fault.
func (c *OpenF1Client) Meetings(ctx context.Context, year int) ([]Meeting, error) {
	var rows []meetingRow
	if err := c.get(ctx, "/meetings", seasonQuery(year), &rows); err != nil {
		return nil, fmt.Errorf("openf1.Meetings(%d): %w", year, err)
	}

	meetings := make([]Meeting, 0, len(rows))
	for _, row := range rows {
		start, err := parseUpstreamTime(row.DateStart)
		if err != nil {
			return nil, fmt.Errorf("openf1.Meetings(%d): meeting %d date_start: %w", year, row.MeetingKey, err)
		}
		meetings = append(meetings, Meeting{
			MeetingKey:   row.MeetingKey,
			Year:         row.Year,
			Name:         row.Name,
			OfficialName: row.OfficialName,
			CircuitKey:   row.CircuitKey,
			CountryName:  row.CountryName,
			Location:     row.Location,
			DateStart:    start,
		})
	}
	return meetings, nil
}

// Sessions returns every Session of a season, as above.
func (c *OpenF1Client) Sessions(ctx context.Context, year int) ([]Session, error) {
	var rows []sessionRow
	if err := c.get(ctx, "/sessions", seasonQuery(year), &rows); err != nil {
		return nil, fmt.Errorf("openf1.Sessions(%d): %w", year, err)
	}

	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		start, err := parseUpstreamTime(row.DateStart)
		if err != nil {
			return nil, fmt.Errorf("openf1.Sessions(%d): session %d date_start: %w", year, row.SessionKey, err)
		}
		end, err := parseUpstreamTime(row.DateEnd)
		if err != nil {
			return nil, fmt.Errorf("openf1.Sessions(%d): session %d date_end: %w", year, row.SessionKey, err)
		}
		sessions = append(sessions, Session{
			SessionKey:  row.SessionKey,
			MeetingKey:  row.MeetingKey,
			CircuitKey:  row.CircuitKey,
			SessionType: row.SessionType,
			SessionName: row.SessionName,
			Year:        row.Year,
			DateStart:   start,
			DateEnd:     end,
			IsCancelled: row.IsCancelled,
		})
	}
	return sessions, nil
}

// WeatherSamples returns one Session's Weather Samples. A Session the upstream holds no weather for
// — a cancelled one, among others — is an empty slice and no error, the same answer shape a season
// it does not carry gets.
func (c *OpenF1Client) WeatherSamples(ctx context.Context, sessionKey int) ([]WeatherSample, error) {
	var rows []weatherRow
	if err := c.get(ctx, "/weather", sessionQuery(sessionKey), &rows); err != nil {
		return nil, fmt.Errorf("openf1.WeatherSamples(%d): %w", sessionKey, err)
	}

	samples := make([]WeatherSample, 0, len(rows))
	for _, row := range rows {
		observedAt, err := parseUpstreamTime(row.Date)
		if err != nil {
			return nil, fmt.Errorf("openf1.WeatherSamples(%d): date: %w", sessionKey, err)
		}
		if row.Rainfall == nil {
			return nil, fmt.Errorf("openf1.WeatherSamples(%d): sample at %s: upstream sent no rainfall",
				sessionKey, observedAt)
		}
		if err := checkSession("weather", sessionKey, row.SessionKey); err != nil {
			return nil, err
		}
		samples = append(samples, WeatherSample{
			SessionKey:       row.SessionKey,
			ObservedAt:       observedAt,
			Rainfall:         *row.Rainfall != 0,
			AirTemperature:   row.AirTemperature,
			TrackTemperature: row.TrackTemperature,
			Humidity:         row.Humidity,
			Pressure:         row.Pressure,
			WindSpeed:        row.WindSpeed,
			WindDirection:    row.WindDirection,
		})
	}
	return samples, nil
}

// SessionResults returns one Session's classification. A Session the upstream holds no results for
// — a Race still to run, and a cancelled one — is an empty slice and no error, the same answer shape
// its weather gets.
func (c *OpenF1Client) SessionResults(ctx context.Context, sessionKey int) ([]SessionResult, error) {
	var rows []sessionResultRow
	if err := c.get(ctx, "/session_result", sessionQuery(sessionKey), &rows); err != nil {
		return nil, fmt.Errorf("openf1.SessionResults(%d): %w", sessionKey, err)
	}

	results := make([]SessionResult, 0, len(rows))
	for _, row := range rows {
		if err := checkSession("session_result", sessionKey, row.SessionKey); err != nil {
			return nil, err
		}
		if row.DriverNumber == nil {
			return nil, fmt.Errorf("openf1.SessionResults(%d): a result row carries no driver_number",
				sessionKey)
		}
		// Absent flags and absent points are shape changes rather than observations: across the
		// 1,661 Race result rows upstream none of the four is ever null, and each of them reads
		// as an ordinary value if guessed — a finish, or a scoreless race.
		if row.DNF == nil || row.DNS == nil || row.DSQ == nil {
			return nil, fmt.Errorf("openf1.SessionResults(%d): driver %d: upstream sent no dnf/dns/dsq",
				sessionKey, *row.DriverNumber)
		}
		if row.Points == nil {
			return nil, fmt.Errorf("openf1.SessionResults(%d): driver %d: upstream sent no points",
				sessionKey, *row.DriverNumber)
		}
		results = append(results, SessionResult{
			SessionKey:   row.SessionKey,
			RacingNumber: *row.DriverNumber,
			Position:     row.Position,
			Points:       *row.Points,
			NumberOfLaps: row.NumberOfLaps,
			DNF:          *row.DNF,
			DNS:          *row.DNS,
			DSQ:          *row.DSQ,
		})
	}
	return results, nil
}

// SessionDrivers returns one Session's entry list — the Racing Numbers of that Session and the names
// they belonged to, which is the only thing upstream that ties a number to a person for a given
// weekend (ADR-0003).
func (c *OpenF1Client) SessionDrivers(ctx context.Context, sessionKey int) ([]SessionDriver, error) {
	var rows []sessionDriverRow
	if err := c.get(ctx, "/drivers", sessionQuery(sessionKey), &rows); err != nil {
		return nil, fmt.Errorf("openf1.SessionDrivers(%d): %w", sessionKey, err)
	}

	drivers := make([]SessionDriver, 0, len(rows))
	for _, row := range rows {
		if err := checkSession("drivers", sessionKey, row.SessionKey); err != nil {
			return nil, err
		}
		if row.DriverNumber == nil || row.FullName == "" {
			return nil, fmt.Errorf("openf1.SessionDrivers(%d): an entry carries no driver_number or "+
				"no full_name, so it resolves nothing", sessionKey)
		}
		drivers = append(drivers, SessionDriver{
			SessionKey:   row.SessionKey,
			RacingNumber: *row.DriverNumber,
			FullName:     row.FullName,
		})
	}
	return drivers, nil
}

// checkSession holds the upstream to the filter it was given. A row for another Session stored under
// this one would leave the Session asked for still empty — and so re-asked on every run, the
// resumption rule defeated silently — or, worse here, attribute one Race's classification to another.
func checkSession(what string, asked, got int) error {
	if got == asked {
		return nil
	}
	return &models.UpstreamError{
		Upstream: UpstreamOpenF1,
		Message:  fmt.Sprintf("%s for session %d answered with a row for session %d", what, asked, got),
	}
}

func seasonQuery(year int) url.Values { return url.Values{"year": {strconv.Itoa(year)}} }

func sessionQuery(sessionKey int) url.Values {
	return url.Values{"session_key": {strconv.Itoa(sessionKey)}}
}

// get performs one paced, bounded request and decodes the array body into out.
//
// The empty-result 404 leaves out untouched — the caller's zero-length slice is the answer — while
// every other non-2xx becomes an UpstreamError carrying the upstream's own status and message.
//
// query rather than a year: a season is selected by `year` and a Session's weather by `session_key`,
// and the pacing, the bound and the 404 discrimination are the same for both.
func (c *OpenF1Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if err := c.pacer.wait(ctx); err != nil {
		return err
	}

	target := c.baseURL + path + "?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &models.UpstreamError{Upstream: UpstreamOpenF1, Message: "request failed", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		message := detailOf(body)
		if resp.StatusCode == http.StatusNotFound && message == noResultsDetail {
			return nil
		}
		return &models.UpstreamError{Upstream: UpstreamOpenF1, Status: resp.StatusCode, Message: message}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &models.UpstreamError{
			Upstream: UpstreamOpenF1,
			Status:   resp.StatusCode,
			Message:  "response was not the expected JSON array",
			Err:      err,
		}
	}
	return nil
}

// detailOf pulls OpenF1's `detail` field out of an error body, falling back to the raw body so an
// upstream answering with something else is still quoted rather than silently blanked.
func detailOf(body []byte) string {
	var envelope struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Detail != "" {
		return envelope.Detail
	}
	return string(body)
}

func parseUpstreamTime(value *string) (time.Time, error) {
	if value == nil || *value == "" {
		return time.Time{}, fmt.Errorf("upstream sent no timestamp")
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return time.Time{}, fmt.Errorf("upstream sent %q: %w", *value, err)
	}
	return t.UTC(), nil
}
