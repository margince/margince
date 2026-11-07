// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package apideprecation holds the public API's notice and removal rules.
package apideprecation

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// WindowMonths is the least time, in calendar months, between a deprecation
// notice and the removal it announces.
const WindowMonths = 3

// NoticeExtension is the OpenAPI extension that carries one operation's Notice.
const NoticeExtension = "x-deprecation"

// PolicyExtension is the OpenAPI extension that states the deprecation policy.
const PolicyExtension = "x-api-deprecation-policy"

// NoticesExtension is the OpenAPI extension that lists every notice.
const NoticesExtension = "x-api-deprecation-notices"

// PolicyLink is the Link header value that points a client at the policy.
const PolicyLink = `<` + httpserver.BaseURL + `/openapi.json>; rel="deprecation"; type="application/json", <https://github.com/margince/margince/blob/develop/docs/reference/api-compatibility.md>; rel="deprecation"; type="text/markdown"`

// Notice records when a release announced an operation's deprecation, and
// the sunset date if one is set.
type Notice struct {
	Release     string `json:"release" yaml:"release"`
	AnnouncedAt string `json:"announced_at" yaml:"announced_at"`
	SunsetAt    string `json:"sunset_at,omitempty" yaml:"sunset_at,omitempty"`
}

// Validate refuses a notice without its release and time, or with a sunset
// earlier than the window allows. The zero Notice is valid.
func (n Notice) Validate() error {
	if n == (Notice{}) {
		return nil
	}
	if n.Release == "" || strings.TrimSpace(n.Release) != n.Release {
		return fmt.Errorf("deprecation release %q must name the announcing release", n.Release)
	}
	announced, err := time.Parse(time.RFC3339, n.AnnouncedAt)
	if err != nil {
		return fmt.Errorf("deprecation must name the announcing release's RFC3339 publication time: %w", err)
	}
	if n.SunsetAt == "" {
		return nil
	}
	sunset, err := time.Parse(time.RFC3339, n.SunsetAt)
	if err != nil {
		return fmt.Errorf("sunset must be an RFC3339 date: %w", err)
	}
	if sunset.Before(EarliestRemoval(announced)) {
		return fmt.Errorf("sunset must be at least %d calendar months after the announcing release", WindowMonths)
	}
	return nil
}

// EarliestRemoval is the first day a removal announced at announced may ship.
func EarliestRemoval(announced time.Time) time.Time {
	announced = announced.UTC()
	first := time.Date(announced.Year(), announced.Month()+WindowMonths, 1, announced.Hour(), announced.Minute(), announced.Second(), announced.Nanosecond(), announced.Location())
	lastDay := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(announced.Day(), lastDay)-1)
}

// Headers are the response headers that announce the notice to a client.
func (n Notice) Headers() (http.Header, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Add("Link", PolicyLink)
	if n.AnnouncedAt == "" {
		return h, nil
	}
	announced, err := time.Parse(time.RFC3339, n.AnnouncedAt)
	if err != nil {
		return nil, err
	}
	h.Set("Deprecation", "@"+strconv.FormatInt(announced.Unix(), 10))
	if n.SunsetAt != "" {
		sunset, err := time.Parse(time.RFC3339, n.SunsetAt)
		if err != nil {
			return nil, err
		}
		h.Set("Sunset", sunset.UTC().Format(http.TimeFormat))
	}
	return h, nil
}

// CheckRemoval refuses removing the operation in release at time at before
// the notice allows it.
func (n Notice) CheckRemoval(release string, at time.Time) error {
	if err := n.Validate(); err != nil {
		return err
	}
	if n.AnnouncedAt == "" {
		return errors.New("record the announcing release and publication date before removing an API")
	}
	if !semver.IsValid(release) || semver.Canonical(release) != release || semver.Prerelease(release) != "" || !strings.HasSuffix(semver.Canonical(release), ".0") {
		return errors.New("API removal requires a stable non-patch release version")
	}
	if release == n.Release || (semver.IsValid(n.Release) && semver.Compare(release, n.Release) <= 0) {
		return errors.New("removal release must follow the announcing release")
	}
	announced, err := time.Parse(time.RFC3339, n.AnnouncedAt)
	if err != nil {
		return err
	}
	earliest := EarliestRemoval(announced)
	if n.SunsetAt != "" {
		sunset, err := time.Parse(time.RFC3339, n.SunsetAt)
		if err != nil {
			return err
		}
		if sunset.After(earliest) {
			earliest = sunset
		}
	}
	if at.Before(earliest) {
		return fmt.Errorf("API removal must wait until %s", earliest.Format(time.RFC3339))
	}
	return nil
}
