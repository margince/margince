// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The sentence under a sync-health row.
//
// The lane sends its condition as `kind` and that condition's facts as
// `detail`, in the producer's own vocabulary: `shed`, `rate_limited`,
// `deals, contacts`. Drawing that raw puts internal words in front of a rep,
// which is why the row drew nothing at all and said only "The CRM sync needs
// attention" — true of every one of the five conditions, and therefore no
// help in telling them apart.
//
// So the words are written HERE, from two closed vocabularies the server owns:
// the five concern kinds, and the value each one carries. A value this build
// does not recognise draws no line rather than its own key, which is what the
// row did for every value before.

import type { MessageKey } from "../i18n/en";

// syncClasses are the incumbent object classes a stale, backfilling or
// overwritten concern names (overlay/incumbent.go). Listed rather than derived
// because they are the INCUMBENT's classes, not ours: "companies" is what a
// connected CRM calls what this product calls organizations, and the row is
// describing that system's sync rather than this one's records.
const syncClasses = [
  "contacts",
  "companies",
  "deals",
  "leads",
  "calls",
  "meetings",
  "emails",
  "notes",
  "tasks",
] as const;

// syncErrors are the sweep failure classes (the contract's own
// `last_sync_error_class`: class only, never the detail, which stays in the
// system log where a support engineer reads it).
const syncErrors = [
  "rate_limited",
  "unreachable",
  "auth",
  "history_gone",
  "internal",
] as const;

// syncBands are the two degraded read-budget bands. `ok` never reaches a row:
// a healthy budget raises no concern.
const syncBands = ["warn", "shed"] as const;

function known<T extends string>(
  vocabulary: readonly T[],
  value: string,
): value is T {
  return (vocabulary as readonly string[]).includes(value);
}

// classList turns "deals, contacts" — the producer's join — back into the
// reader's own words, dropping any class this build does not know rather than
// printing it. A partial list is honest; a list with `widgets` in it is not.
function classList(detail: string, t: (key: MessageKey) => string): string[] {
  return detail
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => known(syncClasses, entry))
    .map((entry) => t(`worklist.sync.class.${entry}` as MessageKey));
}

// syncHealthDetail writes the supporting line for one sync-health row, or
// nothing when it has no fact this build can put into words.
//
// `kind` decides the sentence and `detail` fills it, because the two are one
// answer: `budget_degraded` carries a band and `objects_stale` carries
// classes, and a renderer that read `detail` without its kind could only print
// the value bare — which is the state this replaces.
export function syncHealthDetail(
  kind: string | undefined,
  detail: string | undefined,
  t: (key: MessageKey, vars?: Record<string, string>) => string,
): string | undefined {
  if (!kind || !detail) {
    return undefined;
  }
  switch (kind) {
    case "budget_degraded":
      return known(syncBands, detail)
        ? t(`worklist.sync.band.${detail}` as MessageKey)
        : undefined;
    case "sync_failing":
      return known(syncErrors, detail)
        ? t("worklist.sync.failing", {
            reason: t(`worklist.sync.error.${detail}` as MessageKey),
          })
        : undefined;
    case "objects_stale":
    case "backfill_incomplete":
    case "records_overwritten": {
      const classes = classList(detail, t);
      return classes.length === 0
        ? undefined
        : t(`worklist.sync.${kind}` as MessageKey, {
            classes: classes.join(", "),
          });
    }
    default:
      return undefined;
  }
}
