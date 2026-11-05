// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { INTL_LOCALE } from "../../../format/intllocale";
import type { Locale, Translator } from "../../../i18n";
import { shippedLabel } from "../../../lib/shippedlabels";

type DealCoverage = components["schemas"]["DealCoverage"];
type RecordAssignment = components["schemas"]["RecordAssignment"];

export type CommitteeColleague = Readonly<{
  key: string;
  name: string;
  // The colleague's jobs on the deal; empty for a colleague who holds none.
  jobs: string;
  // The colleague the coverage read finds holding most of the contact.
  carries: boolean;
}>;

/**
 * Our side of the committee: the deal's owner and everyone holding a
 * responsibility on it, one row per colleague or team with all their jobs.
 * The colleague `single_threaded_ours` names is marked, and listed even when
 * they hold no job here, so the finding names somebody on the card.
 */
export function committeeTeam({
  owner,
  assignments,
  coverage,
  t,
  locale,
}: Readonly<{
  owner?: Readonly<{ id: string; name: string }>;
  assignments: readonly RecordAssignment[];
  coverage?: DealCoverage;
  t: Translator;
  locale: Locale;
}>): CommitteeColleague[] {
  const rows = new Map<string, { name: string; jobs: string[] }>();
  const add = (key: string, name: string, job?: string) => {
    const row = rows.get(key) ?? { name, jobs: [] };
    if (job) {
      row.jobs.push(job);
    }
    rows.set(key, row);
  };
  if (owner) {
    add(owner.id, owner.name, t("deal.committee.owner"));
  }
  for (const held of assignments) {
    const role = shippedLabel(
      {
        label: held.role_label || held.role_key,
        label_key: held.role_label_key,
      },
      t,
    );
    add(...assignee(held, t), role);
  }
  const carrier = (coverage?.risks ?? []).find(
    (risk) => risk.kind === "single_threaded_ours",
  )?.user_ids?.[0];
  const carrierName = (coverage?.our_side ?? []).find(
    (colleague) => colleague.user_id === carrier,
  )?.display_name;
  if (carrier && carrierName && !rows.has(carrier)) {
    add(carrier, carrierName);
  }
  const jobs = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "short",
    type: "unit",
  });
  return [...rows].map(([key, row]) => ({
    key,
    name: row.name,
    jobs: jobs.format(row.jobs),
    carries: key === carrier,
  }));
}

// A user is keyed by their id, as the owner and the coverage read key them.
// A team is keyed apart, since its id comes from a different table.
function assignee(
  held: RecordAssignment,
  t: Translator,
): [key: string, name: string] {
  if (held.subject_kind === "team") {
    return [
      `team:${held.subject_id}`,
      `${held.subject_name} ${t("assignments.teamSuffix")}`,
    ];
  }
  const name = held.subject_inactive
    ? `${held.subject_name} ${t("assignments.subjectInactive")}`
    : held.subject_name;
  return [held.subject_id, name];
}
