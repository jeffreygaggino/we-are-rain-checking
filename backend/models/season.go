package models

import (
	"time"

	"github.com/google/uuid"
)

// RaceWeather is one Race's Weather Samples reduced to counts and aggregates. It carries no
// threshold: no Wet Fraction, no Wet Session flag, no wind band. The caller decides what wet and
// windy mean (plans/07-season-mvp.md).
//
// Every aggregate carries its own sample count because `AVG` ignores NULL and `COUNT(*)` does not.
// wind_speed, air_temperature and track_temperature are each independently nullable upstream, so a
// mean over 150 samples of which 40 carry no wind reading is a mean of 110 — reporting it beside a
// count of 150 would state something false. The paired count is what makes the mean readable.
//
// A mean is nil when its count is zero, rather than 0.0, which would read as a still, freezing Race.
type RaceWeather struct {
	SampleCount         int `json:"sampleCount"`
	RainfallSampleCount int `json:"rainfallSampleCount"`

	WindSampleCount  int      `json:"windSampleCount"`
	WindSpeedMeanMps *float64 `json:"windSpeedMeanMps"`
	WindSpeedMaxMps  *float64 `json:"windSpeedMaxMps"`

	AirTemperatureSampleCount int      `json:"airTemperatureSampleCount"`
	AirTemperatureMeanC       *float64 `json:"airTemperatureMeanC"`

	TrackTemperatureSampleCount int      `json:"trackTemperatureSampleCount"`
	TrackTemperatureMeanC       *float64 `json:"trackTemperatureMeanC"`
}

// DriverResult is one Driver's end result in one Race — the Driver-Race, the unit of analysis
// (CONTEXT.md), reported inside the Race it belongs to rather than as a standalone row.
//
// Where they finished, not where they started: Starting Position is #39, deliberately deferred.
//
// Position is nil for a Retirement rather than a sentinel, which would sort into the classification.
// Identity is DriverID; RacingNumber is reported because it is what the Race was run under, and is
// never what a Driver is keyed on (ADR-0003).
type DriverResult struct {
	DriverID     uuid.UUID `json:"driverId"`
	FullName     string    `json:"fullName"`
	ShortName    string    `json:"shortName"`
	RacingNumber int       `json:"racingNumber"`

	Position     *int    `json:"position"`
	Points       float64 `json:"points"`
	NumberOfLaps *int    `json:"numberOfLaps"`
	DNF          bool    `json:"dnf"`
	DNS          bool    `json:"dns"`
	DSQ          bool    `json:"dsq"`
}

// SeasonRace is one Race of a season: where and when it was held, the weather that was recorded, and
// how every Driver finished.
//
// Cancelled is a first-class field rather than a separate list. A Race that was called off is still
// a Race with a date and a name — the season calendar has a slot for it — and what distinguishes it
// is that there was nothing to observe: Weather is nil and Results is empty. Reporting it in its own
// list instead would mean every caller drawing a calendar had to merge two lists back together in
// date order.
//
// Results is never nil, so a caller can range over it without a guard; for a cancelled Race it is
// simply empty.
type SeasonRace struct {
	SessionKey       int       `json:"sessionKey"`
	MeetingKey       int       `json:"meetingKey"`
	RaceName         string    `json:"raceName"`
	DateStart        time.Time `json:"dateStart"`
	CircuitShortName string    `json:"circuitShortName"`
	CountryName      string    `json:"countryName"`
	Location         string    `json:"location"`

	Cancelled bool           `json:"cancelled"`
	Weather   *RaceWeather   `json:"weather"`
	Results   []DriverResult `json:"results"`
}

// Season is one year of Races, in the order they were held.
type Season struct {
	Races []SeasonRace `json:"races"`
}
