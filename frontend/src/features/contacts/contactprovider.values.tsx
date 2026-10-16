// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EvidenceMark } from "../../design-system/evidencemark";
import { type Fact, FactList } from "../../design-system/factlist";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { categoryNames } from "../../screens/provider-categories";
import { useBoughtSource } from "./boughtmarks";
import type { Profile } from "./contactprovider";

/** The receipt: when this was bought, what was asked for, how much of it came
 *  back.
 *
 *  The section showed values and nothing about the transaction, so a reader
 *  could not answer "did my lookup do anything" — a run that returned one
 *  category out of six looked exactly like one that returned all six, and a
 *  value bought months ago looked exactly like one bought a minute ago. */
function RunReceipt({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const run = profile.latest_run;
  if (!run || !profile.retrieved_at) {
    return null;
  }
  const at = formatDateAbbrev(profile.retrieved_at, locale, recordZone);
  const silent = profile.categories_without_answer;
  if (!silent) {
    // The server did not say what went unanswered: an older backend, or an
    // adapter that never declared which claim answers which category. The date
    // is still a fact. A count derived from a missing list would be invented,
    // and "everything came back" is the reading this line exists to prevent.
    return (
      <p className="t-caption">{t("provider.profile.receiptAt", { at })}</p>
    );
  }
  // What was put to the provider, which can be less than what was requested.
  // A fallback that never fired, or a category skipped for want of its
  // prerequisite, was never sent. Counting them as answered would inflate the
  // figure this line reports.
  const asked =
    profile.categories_asked?.length ?? run.requested_categories.length;
  return (
    <p className="t-caption">
      {t("provider.profile.receipt", {
        at,
        asked: formatNumber(asked, locale),
        answered: formatNumber(asked - silent.length, locale),
      })}
    </p>
  );
}

export function ProviderValues({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const facts = useProviderFacts(profile);
  return (
    <>
      <RunReceipt profile={profile} />
      <FactList facts={facts} />
      {(profile.categories_without_answer?.length ?? 0) > 0 && (
        // What was paid for and came back empty: the other half of the
        // question the line below answers. Without it, a run answering one
        // category of six reads as a plain success with five silent blanks.
        // The reader could not tell an empty purchase from a full one.
        <p className="t-caption">
          {t("provider.profile.noAnswer", {
            categories: categoryNames(
              profile.categories_without_answer ?? [],
              t,
            ),
          })}
        </p>
      )}
      {profile.categories_not_requested.length > 0 && (
        // The difference between "we asked and they had nothing" and "we
        // never asked". Without this line a blank field reads as the first
        // when it is often the second.
        <p className="t-caption">
          {t("provider.profile.notRequested", {
            categories: categoryNames(profile.categories_not_requested, t),
          })}
        </p>
      )}
    </>
  );
}

/** Every bought value as one label→value table.
 *
 *  Nine stacked `<div><Eyebrow/>…</div>` blocks before this, each styling
 *  itself: a reader scanning for the mobile number read nine headings down the
 *  card instead of one column of labels. `FactList` is the catalog's primitive
 *  for exactly this shape and it is what every other record surface uses.
 *
 *  Rows are BUILT rather than rendered conditionally, because FactList takes an
 *  array: a fact with no value is left out entirely, and an empty `<dd>` would
 *  claim we know the value and it is blank.
 */
function useProviderFacts(profile: Profile): Fact[] {
  const t = useT();
  const { locale } = useLocale();
  const source = useBoughtSource()(
    profile.provider ?? "provider",
    profile.retrieved_at,
  );
  const mark = (value: string) => (
    <EvidenceMark value={value} source={source} />
  );
  return [
    ...reachFacts(profile, t, locale, mark),
    ...roleFacts(profile, t, mark),
  ];
}

type Translate = ReturnType<typeof useT>;

type Mark = (value: string) => ReactNode;

/** How to reach them: the addresses and numbers somebody paid for, each with
 *  the qualifier that decides whether to trust it. */
function reachFacts(
  profile: Profile,
  t: Translate,
  locale: Locale,
  mark: Mark,
): Fact[] {
  const facts: Fact[] = profile.emails.map((email) => ({
    key: `email-${email.value}`,
    term: t("provider.profile.emails"),
    value: mark(email.value),
    // Which label this is, and whose it is. An address the provider did not
    // classify is labelled from what we asked for, and the note says so.
    note: email.email_type
      ? t(
          email.email_type_source === "provider"
            ? "provider.profile.emailType.provider"
            : "provider.profile.emailType.requested",
          { type: email.email_type },
        )
      : undefined,
  }));
  for (const phone of profile.mobile_phones) {
    facts.push({
      key: `phone-${phone.value}`,
      term: t("provider.profile.mobiles"),
      value: mark(phone.value),
      note:
        phone.confidence == null
          ? undefined
          : t("provider.profile.confidence", {
              percent: formatNumber(Math.round(phone.confidence * 100), locale),
            }),
    });
  }
  if (profile.linkedin_url) {
    facts.push({
      key: "linkedin",
      term: t("provider.profile.linkedin"),
      value: mark(profile.linkedin_url),
    });
  }
  return facts;
}

/** Who they are: where they work now, where they worked before, and the three
 *  attributes the provider files them under. */
function roleFacts(profile: Profile, t: Translate, mark: Mark): Fact[] {
  const facts: Fact[] = [];
  if (profile.current_employment) {
    facts.push({
      key: "employment",
      term: t("provider.profile.employment"),
      value: mark(roleLine(profile.current_employment)),
    });
  }
  for (const job of profile.job_history) {
    facts.push({
      key: `job-${job.company_name}-${job.job_title ?? ""}`,
      term: t("provider.profile.jobHistory"),
      value: mark(roleLine(job)),
    });
  }
  for (const [key, term, value] of [
    ["location", "provider.profile.location", profile.location],
    [
      "departments",
      "provider.profile.departments",
      profile.departments.join(", "),
    ],
    [
      "seniorities",
      "provider.profile.seniorities",
      profile.seniorities.join(", "),
    ],
  ] as const) {
    if (value) {
      facts.push({ key, term: t(term), value: mark(value) });
    }
  }
  return facts;
}

/** A role as one line: what they do, and where. Either half may be absent:
 *  the provider often returns a company with no title, and an unconditional
 *  join would print a leading separator. */
function roleLine(
  role: Readonly<{ job_title?: string | null; company_name?: string | null }>,
): string {
  return [role.job_title, role.company_name].filter(Boolean).join(" · ");
}
