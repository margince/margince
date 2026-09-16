// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account's since-last-visit reading: what changed while the reader was
// away, and whether any of the account was withheld from them.
//
// This file used to also draw the account's open deals as a card of their
// own (CompanyWorkCard). That reading moved into the Deals tab (DealsCard,
// which already carries the same figures) and the rail's own DealsSection, so
// a second list of the same deals is no longer drawn here. hasWorkInFlight
// stays: whether the growth-fit panel stands in for the account's own work
// reading is still a question about the account, asked from both the deals
// and the projects halves of the 360.

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { omitted } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import "./companywork.css";

type Company360 = components["schemas"]["Company360"];
type WorkProject = components["schemas"]["Company360Project"];

/**
 * SinceLastVisit is what moved on this account while the reader was away, and
 * whether any of the account was withheld from them.
 *
 * It is about the ACCOUNT rather than about any one section of it, so it sits
 * in the footer of the account's own 360 card, reached only through
 * sinceLastVisitFooter below.
 *
 * PRIVATE on purpose: the one mount reaches it through sinceLastVisitFooter,
 * and a caller that could put this element in a footer slot directly is the
 * blank-band defect waiting to happen again. It already happened twice.
 *
 * Withheld sections are named ONCE, about the whole page, rather than as a
 * refusal beside each line a reader did not get.
 */
function SinceLastVisit({ view }: Readonly<{ view?: Company360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const since = newActivities(view);
  const first = firstVisit(view);
  const withheld = (view?.sections_omitted?.length ?? 0) > 0;
  if (!speaksSinceLastVisit(view)) {
    return null;
  }
  return (
    <p className="co-work-foot">
      {/* Never both: on a first open the server counts every activity as new,
          and "14 new items" beside "you are opening this account for the
          first time" is the page contradicting itself. */}
      {first && <span className="t-caption">{t("co.since.first")}</span>}
      {!first && since > 0 && (
        <span className="t-caption">
          {plural("co.read.newActivity", since, {
            count: formatNumber(since, locale),
          })}
        </span>
      )}
      {withheld && <span className="t-caption">{t("co.prep.withheld")}</span>}
    </p>
  );
}

// The since-last-visit line for a footer slot, or nothing at all.
//
// UNDEFINED rather than an element that renders null, and that is the whole
// point of this function existing: Panel draws its footer band on the slot's
// truthiness, and an element is truthy whatever it renders, so handing it
// `<SinceLastVisit/>` on an account with nothing to report costs the record a
// blank row. The mount calls this; passing the component straight into a
// footer is the defect it exists to make unavailable.
export function sinceLastVisitFooter(view?: Company360): ReactNode | undefined {
  return speaksSinceLastVisit(view) ? (
    <SinceLastVisit view={view} />
  ) : undefined;
}

// Whether there is a sentence to say at all: read by the component's own
// guard and by the footer helper above, so the band and the sentence cannot
// disagree about whether there is one.
function speaksSinceLastVisit(view?: Company360): boolean {
  return (
    firstVisit(view) ||
    newActivities(view) > 0 ||
    // Optional on BOTH hops: a composite that answered without the omission
    // list names nothing withheld, and reading `.length` off the absent list
    // throws where the whole record page then renders as "this view no longer
    // works".
    (view?.sections_omitted?.length ?? 0) > 0
  );
}

// newActivities is how many landed since the reader's baseline.
//
// Zero and "not counted" are different answers and neither earns a line: a
// withheld section means nobody counted, and a counted zero means nothing
// happened: reporting either as news would be a claim the page cannot make.
function newActivities(view?: Company360): number {
  if (!view || omitted(view, "since_last_visit")) {
    return 0;
  }
  return view.since_last_visit?.new_activities ?? 0;
}

// firstVisit is true only when the account HAS a baseline section and it is
// empty. Read off an absent section it would turn data a reader's grants
// withheld into a claim about their own history.
function firstVisit(view?: Company360): boolean {
  if (!view || omitted(view, "since_last_visit")) {
    return false;
  }
  return Boolean(view.since_last_visit) && !view.since_last_visit?.baseline_at;
}

/**
 * hasWorkInFlight is whether the account has anything moving, which is what
 * decides between the account's own reading and the growth-fit panel in the
 * same slot.
 *
 * A WITHHELD half counts as work: a reader who may not see the projects has
 * not been told this account has none, and swapping in the fit panel would
 * tell them exactly that.
 */
export function hasWorkInFlight(view?: Company360): boolean {
  if (!view) {
    return false;
  }
  const withheld = omitted(view, "deals") || omitted(view, "projects");
  return (
    withheld ||
    (view.deals?.data.length ?? 0) > 0 ||
    liveWork(view.projects).length > 0
  );
}

// The projects the account is about: the ones still in motion. A closed
// project is history, and the same filter the project picker applies: one
// answer to "live", so hasWorkInFlight and the picker cannot disagree.
function liveWork(projects?: readonly WorkProject[]): readonly WorkProject[] {
  return (projects ?? []).filter((project) => project.phase !== "closed");
}
