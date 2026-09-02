package tests_test

import (
	"context"
	"testing"
	"time"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/client"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/tests"
)

// A fixture handed to the stub belongs to the stub from that moment on: SetSeason copies, so a
// caller still holding the slice cannot reach what is already being served.
func TestTheStubServesWhatItWasGivenRatherThanWhatTheCallerLaterWrote(t *testing.T) {
	stub := tests.NewOpenF1Stub(t)

	const year = 2023
	meetings, sessions := seasonFixture(year)
	stub.SetSeason(year, meetings, sessions)

	wantMeeting, wantSession := meetings[0].MeetingName, sessions[0].SessionName
	meetings[0].MeetingName = "written after handing the slice over"
	sessions[0].SessionName = "written after handing the slice over"

	openf1 := client.NewOpenF1Client(stub.BaseURL(), 5*time.Second, 0)

	served, err := openf1.Meetings(context.Background(), year)
	if err != nil {
		t.Fatalf("Meetings: %v", err)
	}
	if served[0].Name != wantMeeting {
		t.Errorf("meeting name = %q, want %q — the stub serves the fixture as it was set",
			served[0].Name, wantMeeting)
	}

	servedSessions, err := openf1.Sessions(context.Background(), year)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if servedSessions[0].SessionName != wantSession {
		t.Errorf("session name = %q, want %q", servedSessions[0].SessionName, wantSession)
	}
}
