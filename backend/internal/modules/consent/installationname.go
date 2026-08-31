// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The installation's own display label, for the public preference page.
//
// It lives in identity, and a module never imports a sibling — so the
// reader is injected and this file holds only the seam.

import (
	"context"
	"log/slog"
)

// InstallationNameReader answers the installation's display name.
type InstallationNameReader interface {
	InstallationName(ctx context.Context) (string, error)
}

// InstallationNameFunc adapts a plain function, so compose can inject
// identity's reader without either module knowing the other.
type InstallationNameFunc func(ctx context.Context) (string, error)

// InstallationName satisfies InstallationNameReader.
func (f InstallationNameFunc) InstallationName(ctx context.Context) (string, error) { return f(ctx) }

// WithInstallationName injects the reader behind the public page's label.
func (s *Store) WithInstallationName(r InstallationNameReader) *Store {
	s.installationName = r
	return s
}

// workspaceName is a display label, so a failure to read it costs the
// label and never the page: somebody trying to stop email must not be
// met with a 500 because the installation has no name on file.
func (s *Store) workspaceName(ctx context.Context) string {
	if s.installationName == nil {
		return ""
	}
	name, err := s.installationName.InstallationName(ctx)
	if err != nil {
		slog.WarnContext(ctx, "preference centre could not read the installation name", "error", err)
		return ""
	}
	return name
}
