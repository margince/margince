// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"net/http"

	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A workspace import does not become personal when its owner loses admin.
// Seed the contact run under the same own-import scope the measured role uses.
func (s sweepSeed) contactImportProfile() importjobtest.Profile {
	s.t.Helper()
	restore := s.asRoles("rep")
	defer restore()
	identity := ids.NewV7().String()
	profile, status := importjobtest.UploadCSV(s.t, s.e, "contact", "full_name,email\nSweep "+identity+","+identity+"@import.test\n")
	if status != http.StatusOK || profile.RunID == "" {
		s.t.Fatalf("uploading an import file → %d %+v", status, profile)
	}
	return profile
}

func (s sweepSeed) testedContactImport() importjobtest.Run {
	s.t.Helper()
	restore := s.asRoles("rep")
	defer restore()
	profile := s.contactImportProfile()
	run, status := importjobtest.CreateRunWithMapping(s.t, s.e, "contact", profile.SourceRef, map[string]string{"full_name": "full_name", "email": "email"})
	if status != http.StatusAccepted && status != http.StatusCreated {
		s.t.Fatalf("starting an import → %d", status)
	}
	return run
}
