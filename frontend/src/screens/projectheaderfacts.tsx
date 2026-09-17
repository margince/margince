// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The project head's facts strip, the same shape dealheaderfacts.tsx and
// companyheaderfacts.tsx draw for their records.
//
// This strip is one of three things that made the project read as a record of
// this product rather than a page of its own, and project360.tsx points here
// for all three. The other two are its frame: the `.record-sheet` wrapper, the
// single raised surface with a hairline that the contact, the account, the lead
// and the deal all stand on — without it the project drew as loose cards on the
// page ground while every record beside it read as a document — and
// `scale="compact"`, the rung those four take, which is also what gives these
// cells a row of their own rather than a column beside the name.
//
// It replaces the running, dot-joined identity line the project alone still
// carried — company · owner · target {date} — while the contact, the account,
// the lead and the deal had all moved to named cells. A reader comparing two
// projects re-parsed a sentence on each; the same facts by position are read
// once. The line's own refusals come with it: a company the grant withholds
// still NAMES its cell, because a project drawn with no company reads as a
// project that has none.

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Fact, RecordFacts } from "../design-system/recordfacts";
import { sectionState } from "../design-system/surfacestate";
import {
  calendarDaysBetween,
  formatDate,
  formatNumber,
} from "../format/format";
import { type Locale, useT } from "../i18n";
import { EntityRef, OwnerName } from "./entityref";

type Project360 = components["schemas"]["Project360"];

/**
 * TargetEndReading is the date a project is due and how far off that is.
 *
 * The count is in whole CALENDAR days, the same reckoning the deal's close
 * uses: "in 3 days" has to mean the third morning from this one, not 72 hours,
 * or a project due on Friday reads as Thursday to anyone working an evening.
 */
function TargetEndReading({
  date,
  locale,
  zone,
}: Readonly<{ date: string; locale: Locale; zone: string }>) {
  const t = useT();
  const days = calendarDaysBetween(new Date(), new Date(date));
  return (
    <span>
      {formatDate(date, locale, zone)}
      {" · "}
      {days < 0
        ? t("project.targetEnd.overdue", {
            days: formatNumber(Math.abs(days), locale),
          })
        : t("project.targetEnd.inDays", { days: formatNumber(days, locale) })}
    </span>
  );
}

/**
 * ProjectIdentityFacts is the head's facts strip: which account the work is
 * for, when it is due and whose project it is — the identity line's own three,
 * in the cells every other record answers its facts in.
 *
 * No phase cell, though a deal names its stage here: the phase is already a tag
 * beside the name AND the ladder under these cells, and a third telling of one
 * fact is what this strip exists to stop.
 *
 * No provenance cell, where a deal carries one: a project has a free-text
 * `source` and no `captured_by`, so a ProvenanceTag here would be a claim about
 * who recorded it that nothing on the row supports.
 */
export function ProjectIdentityFacts({
  view,
  locale,
}: Readonly<{ view: Project360; locale: Locale }>) {
  const t = useT();
  const zone = useRecordZone();
  const project = view.project;
  const company = view.company;
  // Withheld and absent are different answers about a project's account, and
  // the primitive is what keeps them apart — the identity line this replaced
  // made the same distinction and it comes across with it.
  const companyState = sectionState(
    view,
    "company",
    Boolean(company),
    company ? 1 : 0,
  );
  return (
    <RecordFacts>
      <Fact label={t("project.company")}>
        {companyState === "withheld" ? (
          <span data-testid="project-company-withheld">
            {t("state.withheld")}
          </span>
        ) : (
          <EntityRef
            kind="company"
            id={project.company_id}
            name={company?.name}
          />
        )}
      </Fact>
      {/* Always, date or none: when the work is due is a fact a reader checks
          first, and a cell that vanishes while nobody has set one reads as a
          project with no date outstanding rather than one that needs a date. */}
      <Fact label={t("project.targetEnd")}>
        {project.target_end_date ? (
          <TargetEndReading
            date={project.target_end_date}
            locale={locale}
            zone={zone}
          />
        ) : (
          <span className="deal-fact-unset">{t("field.unset")}</span>
        )}
      </Fact>
      <Fact label={t("project.owner")}>
        <OwnerName ownerId={project.owner_id} unowned={t("list.unowned")} />
      </Fact>
    </RecordFacts>
  );
}
