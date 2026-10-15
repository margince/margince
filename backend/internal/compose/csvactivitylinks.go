// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The links cell of an activity row: type:key pairs naming the records the
// activity is filed under. Each key is found by the run's one resolver, so the
// dry run and the commit find the same record, and the file's own rows of
// another type count before they exist.

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// activityLinkTypes are the record types a links cell may name.
var activityLinkTypes = []string{
	migration.ObjectCompany, migration.ObjectContact, migration.ObjectLead, migration.ObjectDeal,
}

// parseLinks reads a links cell into its pairs, in order and without repeats.
// A pair is a type, a colon and a key, and the key may hold colons itself.
// An empty cell names none.
func parseLinks(cell string) ([]linkKey, *migration.Reason) {
	var keys []linkKey
	for part := range strings.SplitSeq(cell, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		object, key, paired := strings.Cut(part, ":")
		object, key = strings.ToLower(strings.TrimSpace(object)), strings.TrimSpace(key)
		if !paired || object == "" || key == "" {
			why := migration.Why(migration.ReasonLinkMalformed, "value", part)
			return nil, &why
		}
		if !slices.Contains(activityLinkTypes, object) {
			why := migration.Why(migration.ReasonLinkTypeUnknown, "value", object, "allowed", strings.Join(activityLinkTypes, ", "))
			return nil, &why
		}
		if link := (linkKey{Object: object, Key: key}); !slices.Contains(keys, link) {
			keys = append(keys, link)
		}
	}
	return keys, nil
}

// activityLinkProblemOf resolves a new activity's links before it lands, under
// the run's broken-link policy. The links are carried into the row's typed cells
// as the records they name. An activity this run already landed keeps the links
// it has: moving it is a relink, not an import.
func (w *csvWriters) activityLinkProblemOf(ctx context.Context, row migration.Row) (rowLink, error) {
	if _, exists, err := w.lookup(ctx, w.object, row.ExternalID); exists || errors.Is(err, errBoundRecordArchived) {
		return rowLink{}, nil
	} else if err != nil {
		return rowLink{}, err
	}
	fields := textFields(row.Fields)
	cell := fields[activityTargetLinks]
	refuse := func(why migration.Reason) rowLink {
		problem := migration.SkippedRow{
			ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(activityTargetLinks),
			Kind: migration.ProblemInvalidValue, Value: cell,
		}.Because(why)
		return rowLink{skip: true, problem: &problem}
	}
	keys, why := parseLinks(cell)
	if why != nil {
		return refuse(*why), nil
	}
	types := w.preset.types()
	for _, key := range keys {
		types = append(types, key.Object)
	}
	kind := strings.ToLower(strings.TrimSpace(fields[activityTargetType]))
	if why := linkTypeRefusal(activities.CheckLinkTypes(kind, types)); why != nil {
		return refuse(*why), nil
	}
	return w.resolveActivityLinks(ctx, row, keys, refuse)
}

// resolveActivityLinks files the row under the run's preset records, then finds
// each pair's record. A row of this upload that has
// not landed yet counts as found in the dry run, and a pair that finds nothing
// is the run's policy: skip the row, or drop that link and land the rest.
func (w *csvWriters) resolveActivityLinks(
	ctx context.Context, row migration.Row, keys []linkKey, refuse func(migration.Reason) rowLink,
) (rowLink, error) {
	preset, missing, err := w.resolvePreset(ctx)
	if err != nil {
		return rowLink{}, err
	}
	if missing != nil {
		return refuse(*missing), nil
	}
	found := slices.Clone(preset)
	pending := 0
	out := rowLink{links: map[string]any{}}
	column := w.headerFor(activityTargetLinks)
	for _, key := range keys {
		answer, err := w.links.resolveLink(ctx, key.Object, key.Key)
		if err != nil {
			return rowLink{}, err
		}
		switch {
		case answer.Found:
			link := activities.ActivityLinkInput{EntityType: key.Object, EntityID: answer.ID}
			if !slices.Contains(found, link) {
				found = append(found, link)
			}
		case answer.Pending:
			pending++
		default:
			broken := w.brokenLink(row, column, key.Object+":"+key.Key, activityLinkWhy(answer.Why))
			if broken.skip {
				return broken, nil
			}
			out.problem = cmp.Or(out.problem, broken.problem)
		}
	}
	if len(found)+pending == 0 {
		return refuse(migration.Why(migration.ReasonActivityUnlinked)), nil
	}
	out.links[activityTargetLinks] = found
	return out, nil
}

// activityLinkWhy words a link that found nothing for an activity. A company
// name two companies share is not about who employs a contact here.
func activityLinkWhy(why migration.Reason) migration.Reason {
	if why.Code == migration.ReasonLinkAmbiguous {
		return migration.Why(migration.ReasonActivityLinkAmbiguous, "name", why.Params["name"])
	}
	return why
}

// linkTypeRefusal says in the report's words why the store's rule on link types
// refused an activity, or is nil when the rule is met.
func linkTypeRefusal(err error) *migration.Reason {
	var why migration.Reason
	switch {
	case err == nil:
		return nil
	case wrapsA[*activities.TooManyLinksError](err):
		why = migration.Why(migration.ReasonTooManyLinks, "max", strconv.Itoa(activities.MaxLinks))
	case wrapsA[*activities.CompanyMeetingError](err):
		why = migration.Why(migration.ReasonCompanyLinkOnMeeting)
	default:
		bad, ok := errors.AsType[*activities.InvalidLinkTypeError](err)
		if !ok {
			return nil
		}
		why = migration.Why(migration.ReasonLinkTypeUnknown, "value", bad.EntityType, "allowed", strings.Join(activityLinkTypes, ", "))
	}
	return &why
}

// holds reports whether err is, or wraps, an error of type T.
func wrapsA[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}
