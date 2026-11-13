// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package licensereport tells an operator the license posture a process
// resolved: the boot log line and the /metrics section.
package licensereport

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// LogPosture writes the one boot line an operator greps for. It names the
// module version, because a refused license and a stale bundled module read
// alike without it. It names the token's origin, because the environment
// outranks the deployment file.
func LogPosture(ctx context.Context, log *slog.Logger, posture licensecheck.Posture, origin string, env runtimeenv.Environment) {
	attrs := []any{"state", string(posture.State), "module", licensecheck.ModuleVersion(), "token_from", origin}
	if posture.Issuer != "" && posture.Issuer != licensecheck.ProductionIssuer {
		// A license minted by a non-production authority. Only a non-production
		// installation accepts one, and an operator reading this log has to be
		// able to tell such an installation from a licensed customer's.
		attrs = append(attrs, "issuer", posture.Issuer)
	}
	if seats, ok := posture.Seats(); ok {
		attrs = append(attrs, "seats", seats)
	}
	if posture.State == licensecheck.StateAbsent && env.IsNonProduction() {
		// A warning, because an unlicensed installation proves nothing about what
		// a licensed one would do.
		log.WarnContext(ctx, "no license configured; this non-production installation is running unlicensed", attrs...)
		return
	}
	if posture.State == licensecheck.StateAbsent {
		log.WarnContext(ctx, "no license configured; this installation runs on its trial and stops working when the trial ends",
			append(attrs, "trial_days", int(licensecheck.TrialLength/(24*time.Hour)))...)
		return
	}
	log.InfoContext(ctx, "license verified", attrs...)
}

// WriteMetrics renders the entitlement section for one posture.
//
// Each state is its own series, and the current one reads 1. A query names the
// state it asks about, so a new state cannot shift an existing threshold.
func WriteMetrics(w io.Writer, posture licensecheck.Posture) {
	var section strings.Builder
	section.WriteString("# HELP margince_license_posture Whether this installation's license verified against the bundled validation module.\n")
	section.WriteString("# TYPE margince_license_posture gauge\n")
	for _, state := range []licensecheck.State{licensecheck.StateValid, licensecheck.StateAbsent, licensecheck.StateRejected} {
		value := 0
		if state == posture.State {
			value = 1
		}
		fmt.Fprintf(&section, "margince_license_posture{state=%s} %d\n", httpserver.Label(string(state)), value)
	}
	// Omitted, not zeroed, when the license caps nothing. Zero seats would read
	// as a license that permits none.
	if seats, ok := posture.Seats(); ok {
		fmt.Fprintf(&section, "# HELP margince_license_seats Full seats the verified license grants.\n"+
			"# TYPE margince_license_seats gauge\n"+
			"margince_license_seats %d\n", seats)
	}
	// Assembled first and written once, so a refused write cannot leave half a
	// gauge family in the exposition.
	//craft:ignore swallowed-errors the renderer httpserver.Metrics takes for this section has no error return; the job section, which does, is a separate parameter that reports its own
	httpserver.WriteLine(w, "%s", section.String())
}
