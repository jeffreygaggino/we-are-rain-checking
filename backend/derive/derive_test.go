package derive_test

import (
	"testing"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/derive"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

func samples(rainfall ...bool) []models.WeatherSample {
	out := make([]models.WeatherSample, 0, len(rainfall))
	for _, r := range rainfall {
		out = append(out, models.WeatherSample{Rainfall: r})
	}
	return out
}

func TestWetFraction(t *testing.T) {
	tests := []struct {
		name string
		in   []models.WeatherSample
		want float64
	}{
		{"no samples is zero, not a division by zero", nil, 0},
		{"empty slice is zero", samples(), 0},
		{"all dry", samples(false, false, false, false), 0},
		{"all wet", samples(true, true, true, true), 1},
		{"half wet", samples(true, false, true, false), 0.5},
		{"one wet sample in ten", samples(true, false, false, false, false, false, false, false, false, false), 0.1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := derive.WetFraction(tc.in); got != tc.want {
				t.Errorf("WetFraction() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A Wet Session "exceeds the documented threshold" (CONTEXT.md), so the comparison is strictly
// greater and a Session sitting exactly on the threshold is dry. Presence of any rain at all is
// explicitly not sufficient — that is the case a naive implementation gets wrong.
func TestIsWetSession(t *testing.T) {
	tests := []struct {
		name string
		frac float64
		want bool
	}{
		{"bone dry", 0, false},
		{"a single sample of rain is not a Wet Session", 0.01, false},
		{"just below the threshold", derive.WetSessionThreshold - 0.001, false},
		{"exactly at the threshold does not exceed it", derive.WetSessionThreshold, false},
		{"just above the threshold", derive.WetSessionThreshold + 0.001, true},
		{"soaked", 1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := derive.IsWetSession(tc.frac); got != tc.want {
				t.Errorf("IsWetSession(%v) = %v, want %v", tc.frac, got, tc.want)
			}
		})
	}
}

// The honesty constraint as a value. Nine wet Races is the measured ceiling of this corpus and it
// cannot grow, so the rainfall axis is expected to sit permanently below MinimumSampleSize.
func TestSampleVerdict(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want derive.Verdict
	}{
		{"nothing at all", 0, derive.VerdictInsufficientSample},
		{"the nine wet Races this corpus contains", 9, derive.VerdictInsufficientSample},
		{"one short of the minimum", derive.MinimumSampleSize - 1, derive.VerdictInsufficientSample},
		{"exactly the minimum is sufficient", derive.MinimumSampleSize, derive.VerdictSufficient},
		{"comfortably above", derive.MinimumSampleSize + 50, derive.VerdictSufficient},
		{"a negative count cannot support a claim", -1, derive.VerdictInsufficientSample},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := derive.SampleVerdict(tc.n); got != tc.want {
				t.Errorf("SampleVerdict(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}

func TestSampleVerdictInsufficient(t *testing.T) {
	if !derive.SampleVerdict(9).Insufficient() {
		t.Error("nine observations should report as insufficient")
	}
	if derive.SampleVerdict(derive.MinimumSampleSize).Insufficient() {
		t.Error("the minimum sample should not report as insufficient")
	}
}

// Bands are lower-inclusive, upper-exclusive, so every speed lands in exactly one band and the
// edges are not silently shared.
func TestWindBand(t *testing.T) {
	tests := []struct {
		name  string
		speed float64
		want  derive.Band
	}{
		{"still air", 0, derive.BandCalm},
		{"just below the calm edge", 1.99, derive.BandCalm},
		{"the calm edge belongs to light", 2, derive.BandLight},
		{"mid light", 3.1, derive.BandLight},
		{"the light edge belongs to moderate", 4, derive.BandModerate},
		{"mid moderate", 5.5, derive.BandModerate},
		{"the moderate edge belongs to strong", 6, derive.BandStrong},
		{"a gale", 24, derive.BandStrong},
		{"a negative reading is not a band", -1, derive.BandUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := derive.WindBand(tc.speed); got != tc.want {
				t.Errorf("WindBand(%v) = %q, want %q", tc.speed, got, tc.want)
			}
		})
	}
}

// Every band a speed can land in must be listed, or the correlation endpoint reports splits that
// silently omit races.
func TestWindBandsCoverEveryBandWindBandReturns(t *testing.T) {
	listed := make(map[derive.Band]bool)
	for _, b := range derive.WindBands() {
		listed[b] = true
	}
	for speed := 0.0; speed < 30; speed += 0.1 {
		if b := derive.WindBand(speed); !listed[b] {
			t.Fatalf("WindBand(%v) = %q, which WindBands() does not list", speed, b)
		}
	}
}
