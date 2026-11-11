// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apideprecation

import (
	"fmt"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	policy "github.com/margince/margince/backend/internal/platform/apideprecation"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// CheckChanges refuses a removal or notice change the deprecation policy does not allow.
func CheckChanges(before, after *openapi3.T, release string, at time.Time) error {
	return CheckChangesRatified(before, after, release, at, nil)
}

// CheckChangesRatified is CheckChanges where a maintainer ratified removing
// the named operations without a published notice.
func CheckChangesRatified(before, after *openapi3.T, release string, at time.Time, ratified RatifiedRemovals) error {
	oldNotices, err := Notices(before)
	if err != nil {
		return err
	}
	newNotices, err := Notices(after)
	if err != nil {
		return err
	}
	if err := checkRecordedNotices(before, after, newNotices, at); err != nil {
		return err
	}
	for path, item := range before.Paths.Map() {
		for method, op := range item.Operations() {
			key := method + " " + httpserver.BaseURL + path
			nextPath := after.Paths.Value(path)
			if nextPath == nil || nextPath.GetOperation(method) == nil {
				if _, ok := ratified[key]; ok {
					continue
				}
				if !op.Deprecated {
					return fmt.Errorf("%s: deprecate the operation in a published release before removal", key)
				}
				if err := oldNotices[key].CheckRemoval(release, at); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
				continue
			}
			if !nextPath.GetOperation(method).Deprecated {
				continue
			}
			oldNotice := oldNotices[key]
			nextNotice := newNotices[key]
			if oldNotice.AnnouncedAt != "" {
				oldAt, err := time.Parse(time.RFC3339, oldNotice.AnnouncedAt)
				if err != nil {
					return err
				}
				nextAt, err := time.Parse(time.RFC3339, nextNotice.AnnouncedAt)
				if err != nil {
					return fmt.Errorf("%s: preserve the published announcement: %w", key, err)
				}
				if nextNotice.Release != oldNotice.Release || !nextAt.Equal(oldAt) {
					return fmt.Errorf("%s: preserve the published announcement release and date", key)
				}
			}
			if oldNotice.SunsetAt != "" {
				oldSunset, err := time.Parse(time.RFC3339, oldNotice.SunsetAt)
				if err != nil {
					return err
				}
				nextSunset, err := time.Parse(time.RFC3339, nextNotice.SunsetAt)
				if err != nil {
					return fmt.Errorf("%s: preserve the published sunset: %w", key, err)
				}
				if nextSunset.Before(oldSunset) {
					return fmt.Errorf("%s: a published sunset must not be brought forward", key)
				}
			}
		}
	}
	return nil
}

func checkRecordedNotices(before, after *openapi3.T, newNotices map[string]policy.Notice, at time.Time) error {
	for path, item := range after.Paths.Map() {
		for method := range item.Operations() {
			notice := newNotices[method+" "+httpserver.BaseURL+path]
			if notice.AnnouncedAt == "" {
				continue
			}
			published, err := time.Parse(time.RFC3339, notice.AnnouncedAt)
			if err != nil {
				return err
			}
			if published.After(at) {
				return fmt.Errorf("%s %s: announcement must name a release already published", method, path)
			}
			priorPath := before.Paths.Value(path)
			if priorPath == nil || priorPath.GetOperation(method) == nil || !priorPath.GetOperation(method).Deprecated {
				return fmt.Errorf("%s %s: mark the operation deprecated before recording its published release", method, path)
			}
		}
	}

	return nil
}
