package models

// HealthOK is the value a reachable dependency reports. There is deliberately no "degraded": a
// dependency this service cannot answer without is either up, or the route is a 503.
const HealthOK = "ok"

// HealthReport is the health route's payload. It names each dependency that was actually checked,
// so a 200 says what it proved rather than only that the process is running.
//
// The forecast cache's hit and miss counters join it in #11, which is the point of it being a
// report rather than a bare status string.
type HealthReport struct {
	Database string `json:"database" example:"ok"`
}
