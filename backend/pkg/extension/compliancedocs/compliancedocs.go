// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package compliancedocs carries the documents a jurisdiction pack ships on
// the published extension surface: the templates, reading copies and records
// an installation under that law reads, downloads and reviews.
//
// A document is inert content. The core serves the selected jurisdiction's
// documents on the admin page and never interprets them.
//
// These types are frozen published API from their first external consumer;
// they evolve additively or through versioned successors, never in place
// (EXT-P3).
//
//margince:extension-surface
package compliancedocs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
)

// Audience is who a document is written for.
type Audience string

const (
	// Employer is the company running the installation, as an employer.
	Employer Audience = "employer"
	// Operator is whoever runs and administers the installation.
	Operator Audience = "operator"
	// Employee is a member of staff whose mail the installation captures.
	Employee Audience = "employee"
)

// Kind is what a document is for.
type Kind string

const (
	// Template is filled in and signed by the installation.
	Template Kind = "template"
	// ReadingCopy is a translation for reading; the original binds.
	ReadingCopy Kind = "reading_copy"
	// Record is a register the installation keeps, such as a processing record.
	Record Kind = "record"
)

// mediaTypes is what the admin page can show or offer for download.
var mediaTypes = map[string]bool{"text/markdown": true, "application/pdf": true}

// Document is one file a pack ships.
type Document struct {
	Jurisdiction jurisdiction.Code
	// ID is the document's stable name within its jurisdiction, used in its
	// download path: lower-case letters, digits and hyphens.
	ID        string
	Title     string
	Language  string
	Audience  Audience
	Kind      Kind
	Body      []byte
	MediaType string
}

// Validate checks one document.
func (d Document) Validate() error {
	if err := d.Jurisdiction.Validate(); err != nil {
		return err
	}
	if d.ID == "" || strings.Trim(d.ID, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return fmt.Errorf("document id %q is not lower-case letters, digits and hyphens", d.ID)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("document %q has no title", d.ID)
	}
	if len(d.Language) != 2 {
		return fmt.Errorf("document %q names language %q; want a two-letter code", d.ID, d.Language)
	}
	switch d.Audience {
	case Employer, Operator, Employee:
	default:
		return fmt.Errorf("document %q names audience %q, which this contract does not define", d.ID, d.Audience)
	}
	switch d.Kind {
	case Template, ReadingCopy, Record:
	default:
		return fmt.Errorf("document %q names kind %q, which this contract does not define", d.ID, d.Kind)
	}
	if len(d.Body) == 0 {
		return fmt.Errorf("document %q is empty", d.ID)
	}
	if !mediaTypes[d.MediaType] {
		return fmt.Errorf("document %q has media type %q; the admin page serves text/markdown and application/pdf", d.ID, d.MediaType)
	}
	return nil
}

// ValidateSet checks every document and refuses a second one under the same
// jurisdiction and ID, because the download path could serve only one of them.
func ValidateSet(docs []Document) error {
	seen := map[string]bool{}
	var errs []error
	for _, d := range docs {
		if err := d.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		key := string(d.Jurisdiction) + "/" + d.ID
		if seen[key] {
			errs = append(errs, fmt.Errorf("document %q is declared twice for jurisdiction %q", d.ID, d.Jurisdiction))
		}
		seen[key] = true
	}
	return errors.Join(errs...)
}
