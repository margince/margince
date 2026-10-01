// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Saying at boot whether this installation knows what the law requires it to
// keep.
//
// The floor fails OPEN: a purge handed no clause shields every row, so a build
// carrying no pack does not destroy correspondence it should have kept — it
// keeps everything, and tells the owner a statute required it. Neither half is
// safe to leave silent, and the one moment an operator reliably reads output
// is the one where the process says what it is.

import (
	"log/slog"

	"github.com/margince/margince/backend/internal/modules/privacy"
)

// announceStatutoryFloor reports the floor posture once, at assembly.
//
// WARN when no floor is known, because it is not a configuration preference:
// mail is being kept that nothing requires keeping, and the reason given for
// keeping it names a law nobody established. INFO when one is known, so the
// line is also the record of WHICH floor this build applies — an installation
// that silently carried the wrong pack would otherwise look identical to one
// carrying the right one.
func announceStatutoryFloor(log *slog.Logger) {
	if log == nil {
		return
	}
	posture := privacy.StatutoryFloorPosture()
	if posture.Known {
		log.Info("privacy: statutory retention floor in force",
			"class", posture.Class, "keep", posture.Keep.String(),
			"from_year_end", posture.FromYearEnd, "packs", posture.Packs)
		return
	}
	// The two absences are different and the message says which. Packs that
	// declined to declare a floor have ANSWERED the question; no packs at all
	// means it was never asked, and only the second is a build mistake.
	if len(posture.Packs) > 0 {
		log.Warn("privacy: no statutory retention floor — the compiled-in jurisdictions declare none, "+
			"so nothing is destroyed on a statutory schedule and a purge reports every kept message as shielded by law",
			"packs", posture.Packs)
		return
	}
	log.Warn("privacy: no statutory retention floor and no jurisdiction packs compiled in — " +
		"captured mail is kept indefinitely and a purge reports it as shielded by law")
}
