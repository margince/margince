// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's coverage findings, as chips a reader scans.
//
// The findings themselves are the server's: /deals/{id}/coverage names each
// rule and says why it fired, and the coverage card below Deal360 already
// renders them in full. This reads THE SAME cached query — the deal page
// mounts both, so the strip costs no second request and cannot disagree with
// the card about what is wrong.
//
// What it adds is the NUMBER. The card's labels are words a reader has to
// trust ("Going cold"); a chip that says "84 days" states the finding itself,
// and a reader can check it against the timeline they are looking at. The
// server sends `days_since_touch` for the rules that have one, so the figure
// is read rather than recomputed.

import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useDealCoverage } from "./deal360/usedealcoverage";
import type { Signal, SignalTone } from "./record360";

type DealCoverageRisk = components["schemas"]["DealCoverageRisk"];

// Every risk is a warning; only the two that mean somebody is GONE are danger.
// A strip where six chips all shout is a strip a rep stops reading — the same
// judgement the coverage card makes, and deliberately the same mapping.
const RISK_TONE: Record<DealCoverageRisk["kind"], SignalTone> = {
  champion_left: "danger",
  stakeholder_left: "danger",
  going_cold: "warn",
  single_threaded_theirs: "warn",
  single_threaded_ours: "warn",
  coverage_gap: "warn",
};

const RISK_LABELS: Record<DealCoverageRisk["kind"], MessageKey> = {
  champion_left: "coverage.risk.champion_left",
  stakeholder_left: "coverage.risk.stakeholder_left",
  going_cold: "coverage.risk.going_cold",
  single_threaded_theirs: "coverage.risk.single_threaded_theirs",
  single_threaded_ours: "coverage.risk.single_threaded_ours",
  coverage_gap: "coverage.risk.coverage_gap",
};

/**
 * useDealSignals reads the deal's coverage findings as scannable chips.
 *
 * The query key is the coverage card's, on purpose: react-query serves both
 * from one entry, so mounting the strip above the card adds no request and the
 * two always read the same findings.
 *
 * Withheld is NOT an empty answer. A caller without the relationship grant is
 * served no findings at all, and rendering that as "nothing is wrong" would
 * report a clean bill of health from a check that never ran. The hook says
 * which of the two happened and lets the caller decide what to draw.
 */
export function useDealSignals(dealId: string, enabled: boolean) {
  const t = useT();
  // The shared read, not a second useQuery over the same key. Three surfaces
  // ask this question and they agreed by luck while each spelled its own; see
  // deal360/usedealcoverage.tsx.
  const { coverage, withheld, ready } = useDealCoverage(dealId, enabled);
  const signals: Signal[] = withheld
    ? []
    : (coverage?.risks ?? []).map((risk) => ({
        key: risk.kind,
        label: t(RISK_LABELS[risk.kind]),
        // The trigger number, when the rule has one. A rule with no figure
        // renders its label alone rather than a blank where a number goes.
        figure:
          risk.days_since_touch != null
            ? t("coverage.daysSinceTouch", { days: risk.days_since_touch })
            : undefined,
        tone: RISK_TONE[risk.kind],
      }));
  return { signals, withheld, ready };
}
