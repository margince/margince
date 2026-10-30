// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func (w *csvWriters) reviewLinks(ctx context.Context, row migration.Row) ([]migration.ReviewLink, error) {
	fields := textFields(row.Fields)
	var links []migration.ReviewLink
	switch w.object {
	case migration.ObjectContact:
		if key := strings.TrimSpace(fields[csvTargetCompanyRef]); key != "" && w.companyRefColumn != "" {
			links = append(links, migration.ReviewLink{Object: migration.ObjectCompany, Key: key})
		} else if key := strings.TrimSpace(fields[csvEmployerName]); key != "" && w.companyNameColumn != "" {
			links = append(links, migration.ReviewLink{Object: migration.ObjectCompany, Key: key, ByName: true})
		}
	case migration.ObjectDeal:
		for _, edge := range []struct{ field, object string }{{csvTargetCompanyRef, migration.ObjectCompany}, {dealTargetContactRef, migration.ObjectContact}} {
			if key := strings.TrimSpace(fields[edge.field]); key != "" && columnFor(w.columns, edge.field) != "" {
				links = append(links, migration.ReviewLink{Object: edge.object, Key: key})
			}
		}
	case migration.ObjectActivity:
		var err error
		links, err = w.reviewActivityLinks(ctx, row, fields)
		if err != nil {
			return nil, err
		}
	}
	for i := range links {
		resolved, err := w.resolveReviewLink(ctx, links[i])
		if err != nil {
			return nil, err
		}
		links[i] = resolved
	}
	return links, nil
}

func (w *csvWriters) resolveReviewLink(ctx context.Context, link migration.ReviewLink) (migration.ReviewLink, error) {
	link.TargetID, link.Pending, link.Dropped = "", false, false
	if link.ByName {
		candidate, err := w.links.employerFor(ctx, employerKey(link.Key))
		if err != nil {
			return link, err
		}
		link.Dropped = candidate.count > 1
		link.Pending = !link.Dropped && candidate.pending && !w.links.plannedEmployerLink(link)
		if !link.Dropped && !candidate.id.IsZero() {
			link.TargetID = candidate.id.String()
		}
		return link, nil
	}
	answer, err := w.links.resolveLink(ctx, link.Object, link.Key)
	if err != nil {
		return link, err
	}
	link.Pending, link.Dropped = answer.Pending && !w.links.plannedEmployerLink(link), !answer.resolves()
	if answer.Found {
		link.TargetID = answer.ID.String()
	}
	return link, nil
}

func (w *csvWriters) reviewActivityLinks(ctx context.Context, row migration.Row, fields map[string]string) ([]migration.ReviewLink, error) {
	links := []migration.ReviewLink{}
	if _, exists, err := w.lookup(ctx, w.object, row.ExternalID); exists || err != nil {
		return nil, err
	}
	keys, why := parseLinks(fields[activityTargetLinks])
	if why != nil {
		return links, nil
	}
	for _, key := range keys {
		links = append(links, migration.ReviewLink{Object: key.Object, Key: key.Key})
	}
	for _, preset := range w.preset.links {
		links = append(links, migration.ReviewLink{Object: preset.Type, Key: preset.ID})
	}
	return links, nil
}
