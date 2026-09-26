// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the machinery did while the reader was away.
//
// A RECEIPT AND A ROUTER, NEVER A SECOND INBOX, which is the endpoint's own
// words and the reason no row here carries a verb. A staged decision is decided
// where decisions are decided and an undo is the record's own; a row may point
// at the surface that holds the verb, and holding it here would put a second
// answer beside the first.
//
// FOUR LANES, because they ask different things: what already happened, what is
// waiting on a human, what was promised and did not land, and what an
// administrator must restore. One list would let a failure sort in beside a
// success, which is the one thing a receipt must never do.
//
// A PLAIN PANEL. The feed above already wears the indigo lead on this page, and
// a second tinted panel is two leads, which is none.
//
// LAST ON THE PAGE, and open — the position the Worklist's own receipt argues
// for: a reader opens Home to find what to do next, and a list of what is
// already finished answers a different question, one worth having and not worth
// leading with.

import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { DataTable } from "../design-system/datatable";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  magicByKey,
  magicConsequenceKey,
  magicSentenceKey,
  magicUndoReasonKey,
  magicWhyKey,
} from "./magic.keys";
import {
  type MagicLane,
  type MagicLine,
  type MagicNotShown,
  type MagicReceipt,
  useMagic,
} from "./magic.queries";
import { sourceUnavailableText } from "./worklist.copy";
import { listReadState } from "./worklist.listread";
import "./brief.css";

// The reading order: what happened, what waits on you, what broke, what needs
// restoring. A list rather than four call sites, so a fifth lane is one entry.
const LANES = [
  "done",
  "needs_you",
  "could_not_complete",
  "watching",
] as const satisfies readonly MagicLane[];

const LANE_HEADING: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.lane.done",
  needs_you: "magic.lane.needsYou",
  could_not_complete: "magic.lane.couldNotComplete",
  watching: "magic.lane.watching",
};

// What there is none OF, per lane. A lane says its own sentence because "no
// decisions are waiting" and "nothing broke" are opposite news and a shared
// "nothing here" would report them as one.
const LANE_EMPTY: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.empty.done",
  needs_you: "magic.empty.needsYou",
  could_not_complete: "magic.empty.couldNotComplete",
  watching: "magic.empty.watching",
};

const NOT_SHOWN_REASON: Readonly<Record<MagicNotShown["reason"], MessageKey>> =
  {
    unadmitted_action: "magic.notShown.unadmittedAction",
    unknown_entity_type: "magic.notShown.unknownEntityType",
    out_of_scope: "magic.notShown.outOfScope",
  };

export function MagicPanel() {
  const t = useT();
  const { locale } = useLocale();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = viewerZone();
  const magic = useMagic();
  const receipt = magic.data;
  const withheld = receipt?.sources_unavailable ?? [];
  return (
    <Panel
      title={t("magic.title")}
      // WHICH WINDOW, in the band that belongs to the whole panel: "nothing
      // happened" over an hour and over a day are different claims, and only
      // the server knows which one this page is making.
      footer={
        receipt?.since &&
        t("magic.since", {
          when: formatDateTime(receipt.since, locale, zone),
        })
      }
    >
      <PanelBody>
        {/* The caveat BEFORE the lanes. A reader who meets it after four
            headings has already read three of them as complete. */}
        <WithheldSources withheld={withheld} />
        {LANES.map((lane) => (
          <MagicLaneSection
            key={lane}
            lane={lane}
            read={magic}
            receipt={receipt}
            // "All clear" is forbidden while a lane could not be read: a lane
            // the reader may not see and a lane with nothing in it are
            // different answers, and only one of them is good news.
            canReportEmpty={withheld.length === 0}
            onRetry={() => void magic.refetch()}
            zone={zone}
          />
        ))}
        <NotShown entries={receipt?.not_shown ?? []} />
      </PanelBody>
    </Panel>
  );
}

function WithheldSources({
  withheld,
}: Readonly<{ withheld: MagicReceipt["sources_unavailable"] }>) {
  const t = useT();
  if (withheld.length === 0) {
    return null;
  }
  return (
    <ul className="magic-withheld">
      {withheld.map((missing) => (
        <li className="t-caption" key={`${missing.source}-${missing.reason}`}>
          {sourceUnavailableText(missing, t)}
        </li>
      ))}
    </ul>
  );
}

/**
 * What this read deliberately left out, by kind and count.
 *
 * Said rather than dropped, because a page showing five lines that never
 * mentions the sixth is a completeness claim nobody made.
 */
function NotShown({
  entries,
}: Readonly<{ entries: readonly MagicNotShown[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  if (entries.length === 0) {
    return null;
  }
  return (
    <ul className="magic-notshown">
      {entries.map((entry) => (
        <li className="t-caption" key={entry.reason}>
          {plural("magic.notShown", entry.count, {
            count: formatNumber(entry.count, locale),
            reason: t(NOT_SHOWN_REASON[entry.reason]),
          })}
        </li>
      ))}
    </ul>
  );
}

function MagicLaneSection({
  lane,
  read,
  receipt,
  canReportEmpty,
  onRetry,
  zone,
}: Readonly<{
  lane: MagicLane;
  read: Readonly<{ isPending: boolean; isError: boolean }>;
  receipt: MagicReceipt | undefined;
  canReportEmpty: boolean;
  onRetry: () => void;
  zone: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const rows = receipt?.[lane];
  // Off the LIST, not off the query's flags alone: most lanes are empty on
  // most days, and a state read from `isPending`/`isError` calls that `ready`
  // and draws a table's header row over no rows.
  const answered = listReadState(read, rows);
  const state =
    answered === "empty" && !canReportEmpty ? "unavailable" : answered;
  // Optional all the way down: a response missing this field is a server older
  // or newer than this client, and a receipt that renders without a count beats
  // a panel that throws and takes the page with it.
  const count = receipt?.totals?.[lane];
  return (
    <section className="magic-lane">
      <div className="magic-lane-head">
        <Eyebrow as="h3">{t(LANE_HEADING[lane])}</Eyebrow>
        {/* THIS PAGE's count, which is all the endpoint promises. The window's
            own size arrives with the cursor that does not exist yet, and a
            figure asserted before then would be neither the page nor the
            window. */}
        {state === "ready" && count !== undefined && (
          <span className="t-caption magic-lane-count">
            {plural("magic.laneCount", count, {
              count: formatNumber(count, locale),
            })}
          </span>
        )}
      </div>
      <SurfaceState
        state={state}
        emptyLabel={t(LANE_EMPTY[lane])}
        loadingLabel={t("magic.loading")}
        detail={{ onRetry }}
      >
        {/* `ready` already means there are rows — the state above is derived
            from the list — so this narrows the type rather than deciding
            anything. */}
        {rows && (
          <DataTable
            label={t(LANE_HEADING[lane])}
            rows={rows}
            // The lanes mint their ids independently, so an id alone can name a
            // row in a lane the reader was not looking at, and React would
            // draw one of the two.
            rowKey={(row: MagicLine) => `${lane}-${row.id}`}
            columns={[
              {
                key: "what",
                header: t("magic.col.what"),
                render: (row: MagicLine) => <LineSentence line={row} />,
              },
              {
                key: "about",
                header: t("magic.col.about"),
                render: (row: MagicLine) => <LineSubject line={row} />,
              },
              {
                key: "by",
                header: t("magic.col.by"),
                render: (row: MagicLine) => <LineBy line={row} />,
              },
              {
                key: "when",
                header: t("magic.col.when"),
                render: (row: MagicLine) => <LineWhen line={row} zone={zone} />,
              },
              {
                key: "wayBack",
                header: t("magic.col.wayBack"),
                render: (row: MagicLine) => <LineWayBack line={row} />,
              },
            ]}
          />
        )}
      </SurfaceState>
    </section>
  );
}

/**
 * What happened, and what it means for the reader.
 *
 * A sentence this build has no key for is DROPPED rather than printed: a newer
 * server mints keys this client predates, and `magic.action.something` on a
 * receipt is worse than a row that says only what it was about and when.
 */
function LineSentence({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const sentence = magicSentenceKey(line.summary.key);
  const consequence = line.consequence
    ? magicConsequenceKey(line.consequence)
    : null;
  // WHY the machinery did it, under what it did. A reason key this build
  // predates draws nothing, for the reason an unknown sentence does.
  const why = line.reason ? magicWhyKey(line.reason.key) : null;
  return (
    <>
      {sentence && <span>{t(sentence, line.summary.values)}</span>}
      {why && line.reason && (
        <p className="t-caption">{t(why, line.reason.values)}</p>
      )}
      {consequence && <p className="t-caption">{t(consequence)}</p>}
    </>
  );
}

/**
 * Who acted: the job as a reader would call it ("Mail filing", "Retention"),
 * never the ledger's internal actor id.
 */
function LineBy({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const key = line.actor.label ? magicByKey(line.actor.label.key) : null;
  return key ? t(key, line.actor.label?.values) : null;
}

/**
 * Which record the line is about, as a link where the app has a page for it.
 *
 * The subject is a COLUMN rather than a word inside the sentence: the server
 * sends it as its own value, and interpolating a record's name into a
 * translated clause is how a sentence ends up ungrammatical in two of three
 * languages.
 */
/**
 * When it happened, and for a watched source how long it has been that way.
 *
 * A watching line's occurred_at is when the condition was OBSERVED, so printing
 * it alone would date every outage to this page load. Where the condition has a
 * beginning the server sends it, and that is the figure a reader acts on.
 */
function LineWhen({ line, zone }: Readonly<{ line: MagicLine; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const began = line.summary.values?.failing_since;
  if (line.lane === "watching" && began) {
    return t("magic.failingSince", {
      when: formatDateTime(began, locale, zone),
    });
  }
  return formatDateTime(line.occurred_at, locale, zone);
}

function LineSubject({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = subjectLabel(line);
  const count = line.count ?? 1;
  // ONE line for a job that touched many records: the most recent one by
  // name, and how many more.
  if (count > 1) {
    return label
      ? plural("magic.aboutMany", count - 1, {
          label,
          others: formatNumber(count - 1, locale),
        })
      : plural("magic.aboutCount", count, {
          count: formatNumber(count, locale),
        });
  }
  if (!label) {
    return t("magic.noRecord");
  }
  const href = recordHref(line);
  return href ? <a href={href}>{label}</a> : label;
}

/**
 * The way back, or why there is none.
 *
 * NO VERB, on this surface as on every other row here. Where a change can be
 * put back, the row says so and points at the record whose history owns the
 * restore; where it cannot, the reason stands in the control's place, because a
 * greyed button with nothing beside it is the shape this field exists to
 * replace.
 */
function LineWayBack({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const undo = line.undo;
  if (!undo) {
    return null;
  }
  if (undo.undoable) {
    const href = recordHref(line);
    const label = t("magic.undo.fromHistory");
    return href ? <a href={href}>{label}</a> : <span>{label}</span>;
  }
  const reason = magicUndoReasonKey(undo.reason);
  // A reason this build has no words for says nothing rather than printing the
  // server's own token: `no_before_image` is not a sentence a reader can act on.
  return reason ? <span className="t-caption">{t(reason)}</span> : null;
}

/**
 * What to call the record this line is about.
 *
 * The entity's own label first, then the caption the approval was staged under,
 * then the mailbox a connector is failing on. Absent rather than invented: a
 * line naming a record it cannot label still says what happened.
 */
function subjectLabel(line: MagicLine): string | undefined {
  const values = line.summary.values;
  return line.entity?.label ?? values?.target ?? values?.account;
}

// The record's address, through the entity registry rather than a switch
// written here: the record types have route names of their own, and a second
// spelling sends a reader to a page that does not exist. An activity resolves
// to nothing on purpose — it is a timeline entry rather than a record with a
// page.
function recordHref(line: MagicLine): string | undefined {
  const entity = line.entity;
  if (!entity || !isEntityKind(entity.type)) {
    return undefined;
  }
  return routeHash(ENTITY[entity.type].route(entity.id));
}
