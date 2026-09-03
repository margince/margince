// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the selected row is ABOUT, beside the queue.
//
// A row says why it is on the page. It cannot say what else is true of the
// person or deal behind it — whether they have other open work, when anybody
// last spoke to them, what the account is worth — and a rep deciding how to
// answer needs that. Today the only way to see it is to leave the queue, which
// costs the reader their place in it.
//
// So the record's own 360 read is drawn beside the list. It is the SAME read
// the record page makes, not a second assembly of the same facts: a pane that
// composed its own view of a person would be a second answer to "what do we
// know about them", and the two would drift.

import { Panel } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { usePerson360 } from "./person360";
import type { WorklistItem } from "./worklist.queries";

// The pane, for whichever record the selected row is about.
//
// Only a PERSON is drawn today. A deal-bearing row already carries its own
// figures — amount, close date, owner, risk evidence — on the row itself, so a
// pane repeating them would be the second spelling this file exists to avoid;
// what a deal row lacks is its timeline, and that is its own change. A row
// about neither draws nothing rather than an empty frame.
export function WorklistPane({ item }: Readonly<{ item: WorklistItem }>) {
  const subject = item.subject;
  if (subject?.type !== "person") {
    return null;
  }
  return <PersonContext id={subject.id} label={subject.label} />;
}

// Whether this row HAS a pane, asked before one is rendered.
//
// A component returning null is still an element, and an element handed to
// PageZones still gets its aside column and its landmark. The caller has to be
// able to ask the question without rendering the answer, so the rule lives
// here — beside the component that obeys it — rather than in the screen.
export function hasPane(item: WorklistItem | undefined): boolean {
  return item?.subject?.type === "person";
}

// One person's context: who they are, and what else is open with them.
function PersonContext({
  id,
  label,
}: Readonly<{ id: string; label?: string }>) {
  const t = useT();
  const view = usePerson360(id);
  const state = view.isPending
    ? "loading"
    : view.isError
      ? "failed"
      : ("ready" as const);
  return (
    <Panel title={label ?? t("worklist.pane.title")}>
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.pane.nothing")}
        loadingLabel={t("worklist.pane.loading")}
        detail={{ onRetry: () => void view.refetch() }}
      >
        {view.data && <PersonFacts view={view.data} />}
      </SurfaceState>
    </Panel>
  );
}

// The facts a rep answering this row would otherwise open a second page for.
//
// Deliberately few, and chosen for the question the queue leaves open: a row
// says a customer is waiting, and what it cannot say is how long the silence
// has run in BOTH directions. A rep who last wrote yesterday answers
// differently from one who has not written since March.
//
// A pane that reproduced the record page would be the record page in a
// narrower column, and the reader who wanted that has a link on the row.
function PersonFacts({
  view,
}: Readonly<{ view: NonNullable<ReturnType<typeof usePerson360>["data"]> }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <dl className="worklist-pane-facts">
      <dt className="t-meta">{t("worklist.pane.lastInbound")}</dt>
      <dd className="t-body">
        {spoken(view.last_inbound_at, t, locale, zone)}
      </dd>
      <dt className="t-meta">{t("worklist.pane.lastOutbound")}</dt>
      <dd className="t-body">
        {spoken(view.last_outbound_at, t, locale, zone)}
      </dd>
    </dl>
  );
}

// When somebody last wrote, or that nobody has.
//
// "Never" is a reading rather than a gap: a customer nobody has ever answered
// is the strongest case on the page for answering now, and an em dash would
// leave the reader to guess whether the fact was missing or the silence real.
function spoken(
  at: string | null | undefined,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string {
  return at ? formatDateTime(at, locale, zone) : t("worklist.pane.never");
}
