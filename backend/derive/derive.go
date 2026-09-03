// Package derive holds this project's decision functions: value in, value out, no database.
//
// They live here rather than in a service because each one encodes a project decision rather than a
// mechanic. Changing the Wet Session threshold should be one edit with one failing test, not an
// exercise in seeding an exact database state.
package derive

import "github.com/jeffreygaggino/we-are-rain-checking/backend/models"

// WetSessionThreshold is the Wet Fraction a Session must *exceed* to be a Wet Session — the
// comparison is strictly greater, so a Session sitting exactly here is dry.
//
// A quarter of a Session's samples means rain was a condition of the Race rather than an incident
// during it. The presence of any rain at all is deliberately not sufficient: this source's Rainfall
// flag trips on a single sample, and treating that as a wet Race is how a dry weekend with one
// passing shower ends up in the wet bucket.
const WetSessionThreshold = 0.25

// MinimumSampleSize is the honesty constraint as a value. Below roughly thirty observations the
// standard error on a rate is wider than any effect these seasons could show, so a split computed
// from fewer is reported as Insufficient Sample rather than as a number.
//
// The measured wet total is nine Races across four seasons and it cannot grow — the upstream has
// no data before 2023 — so the rainfall axis is expected to sit below this permanently. That is the
// intended output, not a gap to close.
const MinimumSampleSize = 30

// Verdict is whether a sample can support a claim at all. It is a first-class result, never an
// error and never an omission.
type Verdict string

const (
	VerdictInsufficientSample Verdict = "insufficient_sample"
	VerdictSufficient         Verdict = "sufficient"
)

func (v Verdict) Insufficient() bool { return v == VerdictInsufficientSample }

// Band is a wind speed partitioned into one of four ranges.
type Band string

const (
	BandUnknown  Band = "unknown"
	BandCalm     Band = "calm"
	BandLight    Band = "light"
	BandModerate Band = "moderate"
	BandStrong   Band = "strong"
)

// Wind band edges in metres per second. Both upstreams are read in m/s — OpenF1 reports it and
// Open-Meteo is asked for it — so no conversion exists here to get wrong.
//
// Lower-inclusive, upper-exclusive, so a speed lands in exactly one band.
const (
	bandLightFloor    = 2.0
	bandModerateFloor = 4.0
	bandStrongFloor   = 6.0
)

// WindBands lists every band WindBand can return for a valid speed, in ascending order. The
// correlation endpoint reports a split per band, so a band missing from this list would drop its
// Races out of the report silently.
func WindBands() []Band {
	return []Band{BandCalm, BandLight, BandModerate, BandStrong}
}

// WetFraction is the proportion of a Session's Weather Samples recording Rainfall.
//
// No samples returns zero rather than dividing by zero. That is not a claim the Session was dry —
// IsWetSession would say so — it is the absence of evidence, which SampleVerdict is what reports.
func WetFraction(samples []models.WeatherSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	wet := 0
	for _, s := range samples {
		if s.Rainfall {
			wet++
		}
	}
	return float64(wet) / float64(len(samples))
}

// IsWetSession reports whether a Wet Fraction exceeds the threshold.
func IsWetSession(fraction float64) bool { return fraction > WetSessionThreshold }

// SampleVerdict reports whether n observations can support a claim.
func SampleVerdict(n int) Verdict {
	if n < MinimumSampleSize {
		return VerdictInsufficientSample
	}
	return VerdictSufficient
}

// WindBand partitions a wind speed in m/s into a band. A negative speed is not a reading, so it
// bands as unknown rather than folding into calm.
func WindBand(speed float64) Band {
	switch {
	case speed < 0:
		return BandUnknown
	case speed < bandLightFloor:
		return BandCalm
	case speed < bandModerateFloor:
		return BandLight
	case speed < bandStrongFloor:
		return BandModerate
	default:
		return BandStrong
	}
}
