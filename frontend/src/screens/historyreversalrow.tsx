import { type ReactNode, useId, useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { Button } from "../design-system/atoms";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { entryFieldChanges } from "./history.logic";
import { HistoryFieldDiff } from "./historyfielddiff";
import { historyFieldLabel } from "./historyfieldlabels";
import { netChanges, type PairRow } from "./historyreversal";
import { historyValue } from "./historyvalues";

// The reversal and the change it reversed, drawn as ONE line the reader can
// open.
//
// Collapsing is a DISCLOSURE and never a deletion: this is the surface whose
// whole job is showing what happened, so both audit rows stay reachable — by a
// screen reader too, which is why the control is a real button announcing its
// region rather than a styled caret over hidden content.

// The headline for one pair, which is four sentences rather than one.
//
// It turns on two facts and both change what is TRUE, not merely how it reads:
// whether anything survived the reversal, and whether the two actors are one
// person. "Sam's change, undone by Sam" describes two parties who happen to
// share a name, and a pair that says "undone" while a field still holds a new
// value is the one outcome worse than showing both rows.
function headlineKey(row: PairRow): MessageKey {
  if (row.whollyUndone) {
    return row.sameActor
      ? "history.reversal.collapsedSelf"
      : "history.reversal.collapsed";
  }
  return row.sameActor
    ? "history.reversal.partlySelf"
    : "history.reversal.partly";
}

// The name a headline can put in front of a reader.
//
// A row whose actor no longer resolves to a seat carries no name, and the
// headline is a SENTENCE about two people — so it takes the same phrase the
// audit list already uses rather than leaving a gap where a person belongs or
// printing an id nobody can act on.
export function actorName(
  name: string | null | undefined,
  t: (key: MessageKey) => string,
): string {
  return name ?? t("audit.unknownMember");
}

// What a pair whose every field went back has left to show: the value the
// record holds NOW, once, and the fact that the round trip came to nothing.
//
// Not a diff. There is no movement to point an arrow at, and drawing `C → B`
// would invite reading the reversal as a change that stands.
function SettledFace({
  row,
  currency,
}: Readonly<{ row: PairRow; currency: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const values = { currency, locale, zone };
  return (
    <>
      <ul className="entry-fields">
        {entryFieldChanges(row.reversal).map((change) => (
          <li key={change.field} className="entry-field">
            <span className="entry-field-name">
              {historyFieldLabel(change.field, t)}
            </span>
            <span>
              {historyValue(change.field, change.newValue, values) ??
                t("history.cleared")}
            </span>
          </li>
        ))}
      </ul>
      <span className="reversal-net">{t("history.reversal.net")}</span>
    </>
  );
}

// What a pair left behind, on the collapsed face.
//
// Computed over this viewer's own masked images, so it claims only what this
// reader can see — and shown as a diff, because a residual IS a movement that
// still stands.
function ResidualFace({
  row,
  currency,
}: Readonly<{ row: PairRow; currency: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const values = { currency, locale, zone };
  return (
    <>
      <span className="reversal-net">{t("history.reversal.stillChanged")}</span>
      <ul className="entry-fields">
        {netChanges(row).map((change) => (
          <li key={change.field} className="entry-field">
            <span className="entry-field-name">
              {historyFieldLabel(change.field, t)}
            </span>
            <HistoryFieldDiff
              field={change.field}
              oldValue={change.oldValue}
              newValue={change.newValue}
              values={values}
            />
          </li>
        ))}
      </ul>
    </>
  );
}

export function ReversalPairRow({
  row,
  currency,
  children,
}: Readonly<{
  row: PairRow;
  currency: string | null | undefined;
  // The two rows whole, built by the caller from the same row component every
  // other entry uses. Passed in rather than rendered here so the expansion is
  // the ordinary history row — its own actor, time, diff and verb — and not a
  // second, thinner spelling of one.
  children: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [open, setOpen] = useState(false);
  const regionId = useId();
  const words = {
    actor: actorName(row.reversed.actor_name, t),
    undoer: actorName(row.reversal.actor_name, t),
  };
  return (
    <li className="reversal-pair">
      <span className="tl-body">
        <span className="tl-title">{t(headlineKey(row), words)}</span>
        <span className="tl-meta">
          {/* The REVERSAL's time: the pair sits where its newest member sits,
              so nothing appears to have happened earlier than it did. */}
          <span>{formatDateTime(row.atIso, locale, recordZone)}</span>
        </span>
        {row.whollyUndone ? (
          <SettledFace row={row} currency={currency} />
        ) : (
          <ResidualFace row={row} currency={currency} />
        )}
        {/* The only control on the collapsed face, and it writes nothing. Two
            changes with opposite intents have no honest single label, so the
            verbs live on the rows they belong to, inside. */}
        <Button
          small
          variant="ghost"
          className="reversal-toggle"
          aria-expanded={open}
          aria-controls={open ? regionId : undefined}
          onClick={() => setOpen((was) => !was)}
        >
          {t(open ? "history.reversal.collapse" : "history.reversal.expand")}
        </Button>
        {open && (
          <ul id={regionId} className="timeline reversal-pair-body">
            {children}
          </ul>
        )}
      </span>
    </li>
  );
}
