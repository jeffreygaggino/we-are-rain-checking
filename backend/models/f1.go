package models

import (
	"time"

	"github.com/google/uuid"
)

// Circuit is seeded, not ingested: coordinates are owned by this repo rather than geocoded, and the
// id is a literal constant so the same Circuit is the same id in every environment (ADR-0003).
type Circuit struct {
	ID          uuid.UUID `db:"id"           json:"id"`
	CircuitKey  int       `db:"circuit_key"  json:"circuitKey"`
	ShortName   string    `db:"short_name"   json:"shortName"`
	Location    string    `db:"location"     json:"location"`
	CountryName string    `db:"country_name" json:"countryName"`
	Latitude    float64   `db:"latitude"     json:"latitude"`
	Longitude   float64   `db:"longitude"    json:"longitude"`
	CreatedAt   time.Time `db:"created_at"   json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at"   json:"updatedAt"`
}

// Driver is seeded. FullName is the upstream display string ingest resolves on; it is the closest
// thing to a stable identifier upstream offers, which is why identity originates here instead.
type Driver struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	FullName  string    `db:"full_name"  json:"fullName"`
	ShortName string    `db:"short_name" json:"shortName"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

// Meeting is a Grand Prix weekend at one Circuit, spanning several Sessions.
type Meeting struct {
	MeetingKey   int       `db:"meeting_key"   json:"meetingKey"`
	Year         int       `db:"year"          json:"year"`
	Name         string    `db:"name"          json:"name"`
	OfficialName string    `db:"official_name" json:"officialName"`
	CircuitID    uuid.UUID `db:"circuit_id"    json:"circuitId"`
	CountryName  string    `db:"country_name"  json:"countryName"`
	Location     string    `db:"location"      json:"location"`
	DateStart    time.Time `db:"date_start"    json:"dateStart"`
	CreatedAt    time.Time `db:"created_at"    json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updatedAt"`
}

// SessionNameRace is the one Session name that awards championship points to a full grid. Sprint is
// deliberately excluded: every claim this service makes is per Race.
const SessionNameRace = "Race"

type Session struct {
	SessionKey  int       `db:"session_key"  json:"sessionKey"`
	MeetingKey  int       `db:"meeting_key"  json:"meetingKey"`
	CircuitID   uuid.UUID `db:"circuit_id"   json:"circuitId"`
	SessionType string    `db:"session_type" json:"sessionType"`
	SessionName string    `db:"session_name" json:"sessionName"`
	Year        int       `db:"year"         json:"year"`
	DateStart   time.Time `db:"date_start"   json:"dateStart"`
	DateEnd     time.Time `db:"date_end"     json:"dateEnd"`
	IsCancelled bool      `db:"is_cancelled" json:"isCancelled"`
	CreatedAt   time.Time `db:"created_at"   json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at"   json:"updatedAt"`
}

// WeatherSample is one timestamped observation at the Circuit during a Session.
//
// Rainfall is a binary presence flag. This source carries no intensity — across all four seasons
// the only values present are 0 and 1 — so drizzle and a downpour are the same observation. There
// is deliberately no field here implying a magnitude.
type WeatherSample struct {
	SessionKey       int       `db:"session_key"       json:"sessionKey"`
	ObservedAt       time.Time `db:"observed_at"       json:"observedAt"`
	Rainfall         bool      `db:"rainfall"          json:"rainfall"`
	AirTemperature   *float64  `db:"air_temperature"   json:"airTemperature"`
	TrackTemperature *float64  `db:"track_temperature" json:"trackTemperature"`
	Humidity         *float64  `db:"humidity"          json:"humidity"`
	Pressure         *float64  `db:"pressure"          json:"pressure"`
	WindSpeed        *float64  `db:"wind_speed"        json:"windSpeed"`
	WindDirection    *int      `db:"wind_direction"    json:"windDirection"`
	CreatedAt        time.Time `db:"created_at"        json:"createdAt"`
	UpdatedAt        time.Time `db:"updated_at"        json:"updatedAt"`
}

// SessionResult is one Driver's classification in one Session.
//
// RacingNumber is stored because it is what the upstream row carried, but it is not identity: it
// belongs to a season, and number 1 follows the championship. DriverID is the key (ADR-0003).
// Position is nil for a Retirement — a sentinel would sort into the classification.
type SessionResult struct {
	SessionKey   int       `db:"session_key"    json:"sessionKey"`
	DriverID     uuid.UUID `db:"driver_id"      json:"driverId"`
	RacingNumber int       `db:"racing_number"  json:"racingNumber"`
	Position     *int      `db:"position"       json:"position"`
	Points       float64   `db:"points"         json:"points"`
	NumberOfLaps *int      `db:"number_of_laps" json:"numberOfLaps"`
	DNF          bool      `db:"dnf"            json:"dnf"`
	DNS          bool      `db:"dns"            json:"dns"`
	DSQ          bool      `db:"dsq"            json:"dsq"`
	CreatedAt    time.Time `db:"created_at"     json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at"     json:"updatedAt"`
}
