// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words for the values an analytics dimension holds, where the product
// already names that value somewhere. The report cards read the two maps
// below; a question's answer reads all of them through `valueLabel`. A value
// no screen names yet keeps its wire spelling rather than gaining a new word
// here.

import { dealStatusLabel } from "../../components/dealdials";
import { isMessageKey, type useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { writtenByLabel } from "../../lib/leadorigins";
import { LEAD_STATUS_FILTER_OPTIONS } from "../leads/leadpresentation";
import { isProjectPhase, PHASE_LABEL } from "../projects/projects.form";

// forecast_category dimension values (report.go's forecastCategoryExpr):
// the four the deal itself can carry, plus the server-derived "slipped" —
// a claimed commit/best_case deal whose close date is past, missing, or
// still provisional. Omitting it here doesn't shrink the
// total; it moves the deal's amount into no tile at all.
export const FORECAST_CATEGORIES = [
  { key: "commit", labelKey: "deal.fcCommit" },
  { key: "best_case", labelKey: "deal.fcBestCase" },
  { key: "pipeline", labelKey: "deal.fcPipeline" },
  { key: "omitted", labelKey: "deal.fcOmitted" },
  { key: "slipped", labelKey: "deal.fcSlipped" },
] as const;

// The current standing a meeting can hold, in the order a week reads: what is
// ahead, what happened, what did not, what was called off. A hand-kept mirror
// of the server's CHECK vocabulary — a status the server grows is absent here
// until this list learns it, rather than mislabeled.
export const MEETING_STATUSES = [
  { key: "booked", labelKey: "analytics.meetingsBooked" },
  { key: "held", labelKey: "analytics.meetingsHeld" },
  { key: "no_show", labelKey: "analytics.meetingsNoShow" },
  { key: "canceled", labelKey: "analytics.meetingsCanceled" },
] as const;

const LEAD_ENTITIES: ReadonlySet<string> = new Set(["leads-by-status"]);

function keyed(
  pairs: readonly Readonly<{ key: string; labelKey: MessageKey }>[],
  value: string,
): MessageKey | undefined {
  return pairs.find((pair) => pair.key === value)?.labelKey;
}

function valueKey(
  entity: string,
  field: string,
  value: string,
): MessageKey | undefined {
  switch (field) {
    case "status":
      return LEAD_ENTITIES.has(entity)
        ? LEAD_STATUS_FILTER_OPTIONS.find((option) => option.value === value)
            ?.label
        : dealStatusLabel(value);
    case "forecast_category":
      return keyed(FORECAST_CATEGORIES, value);
    case "meeting_status":
      return keyed(MEETING_STATUSES, value);
    case "phase":
      return isProjectPhase(value) ? PHASE_LABEL[value] : undefined;
    case "kind": {
      const key = `timeline.kind.${value}`;
      return isMessageKey(key) ? key : undefined;
    }
    case "loss_reason": {
      // A built-in reason reads in the reader's language; one an
      // administrator added keeps its stored key.
      const key = `lossReasons.builtin.${value}`;
      return isMessageKey(key) ? key : undefined;
    }
    case "loss_reason_state": {
      const key = `analytics.lossState.${value}`;
      return isMessageKey(key) ? key : undefined;
    }
    default:
      return undefined;
  }
}

/**
 * A dimension value in the reader's words, or null when no screen names it:
 * the caller then shows the wire value, which is what it is called everywhere
 * else it appears.
 */
export function valueLabel(
  t: ReturnType<typeof useT>,
  entity: string,
  field: string,
  value: string,
): string | null {
  if (LEAD_ENTITIES.has(entity)) {
    if (field === "source") return writtenByLabel(value, t);
    if (field === "origin") return spoken(value);
  }
  const key = valueKey(entity, field, value);
  return key ? t(key) : null;
}

// A lead origin key as words: the report holds the key, not the list's label.
function spoken(key: string): string {
  const words = key.replace(/[_-]+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
}
