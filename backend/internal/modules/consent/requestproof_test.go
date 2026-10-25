// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"
	"time"
)

func TestAProofIsKeptThreeYearsPastTheEndOfTheYearItWasAnsweredIn(t *testing.T) {
	cases := []struct {
		name     string
		answered time.Time
		want     time.Time
	}{
		{"the first day of a year", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"the last morning of a year", time.Date(2026, 12, 31, 9, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"an answer where the new year has begun", time.Date(2026, 12, 31, 11, 0, 0, 0, time.UTC), time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"the next day", time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC), time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"the same instant carried in another zone", time.Date(2026, 12, 31, 20, 0, 0, 0, time.FixedZone("UTC-10", -10*60*60)), time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := ProofKeepUntil(c.answered); !got.Equal(c.want) {
			t.Errorf("%s: answered %s is kept until %s, want %s", c.name, c.answered, got, c.want)
		}
	}
}

func TestTheRequesterAddressComesFromTheOfficerThenTheLinkThenTheReference(t *testing.T) {
	stated, mailed := "Officer@Stated.test", "link@mailed.test"
	cases := []struct {
		name             string
		stated, mailedTo *string
		subjectRef, want string
	}{
		{"the officer's address wins", &stated, &mailed, "ref@subject.test", "officer@stated.test"},
		{"then the address the link went to", nil, &mailed, "ref@subject.test", "link@mailed.test"},
		{"then a reference that is an address", nil, nil, "ref@subject.test", "ref@subject.test"},
		{"a contact id is no address", nil, nil, "0190a1b2-0000-7000-8000-000000000000", ""},
	}
	for _, c := range cases {
		got, err := requesterAddress(c.stated, c.mailedTo, c.subjectRef)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if _, err := requesterAddress(new("Ada <ada@x.test>"), nil, ""); err == nil {
		t.Error("a stated address with a display name was accepted")
	}
}
