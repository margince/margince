import { type ReactNode, useState } from "react";
import { Badge, Button, Radio } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { AttentionItem } from "./today.queries";
import "./today.merge.css";

// The merge decision, made where it is asked.
//
// This card exists because the surface it replaces did not make it: a row said
// "two companies look like the same one — 93% match" and a button took the
// reader to another screen to find out which two. A person cannot answer that
// question from a percentage, so the percentage was the whole of what they were
// given and the decision was somewhere else.
//
// What a person actually needs is here: both records by name, what each one
// carries, and the fields the detector compared. The verb is a choice of
// winner, because that is the shape of the decision — not accept or reject.

type Pair = NonNullable<AttentionItem["pair"]>;
type Side = Pair["left"];
type Evidence = Pair["evidence"][number];

// The comparison vocabulary, in the reader's own language.
//
// A Map rather than an object: `get` answers `MessageKey | undefined` with no
// cast, and no prototype key can answer for a field the server never sent. The
// server already drops any field this map has no word for, so a miss here means
// the two lists have drifted — the row is skipped rather than printed raw,
// which is the leak the old evidence table shipped with.
const FIELD_LABELS = new Map<string, MessageKey>([
  ["display_name", "day.merge.fieldDisplayName"],
  ["legal_name", "day.merge.fieldLegalName"],
  ["full_name", "day.merge.fieldName"],
  ["email", "day.merge.fieldEmail"],
  ["phone", "day.merge.fieldPhone"],
  ["matched_lane", "day.merge.fieldMatchedLane"],
  ["channel_identity", "day.merge.fieldChannel"],
]);

const SIGNAL_LABELS = new Map<string, MessageKey>([
  ["agree", "day.merge.signalAgree"],
  ["collide", "day.merge.signalCollide"],
  ["one_sided", "day.merge.signalOneSided"],
]);

// One record, as a choosable side.
//
// The whole tile is the label of its radio, so the target is the card a reader
// is already looking at rather than a 16px circle beside it. What each side
// CARRIES leads the detail line: a company with twelve contacts and one with
// three is the clearest signal available for which record is the real one, and
// it is also exactly what a merge moves.
function SideChoice({
  side,
  name,
  checked,
  onPick,
  disabled,
}: Readonly<{
  side: Side;
  name: string;
  checked: boolean;
  onPick: () => void;
  disabled: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // `Radio` draws its own label element, so the tile is a div: a label nested
  // in a label is invalid, and the browser gives the click to whichever it
  // resolves first rather than to the choice the reader aimed at.
  return (
    <div className="merge-side" data-picked={checked ? "" : undefined}>
      <Radio
        name={name}
        checked={checked}
        onChange={onPick}
        disabled={disabled}
        label={
          <span className="merge-side-body">
            <span className="t-body merge-side-name">{side.label}</span>
            {side.detail && (
              <span className="t-caption merge-side-detail">{side.detail}</span>
            )}
            {/* Only when it counts something. This line exists to say which
                side is the real record, and "carries 0 related records" is a
                row of text that answers nothing — on a fresh workspace it
                appeared under BOTH sides and made the comparison longer
                without making it more decisive. */}
            {side.related_count != null && side.related_count > 0 && (
              <span className="t-caption merge-side-detail">
                {t("day.merge.carries", {
                  count: formatNumber(side.related_count, locale),
                })}
              </span>
            )}
          </span>
        }
      />
    </div>
  );
}

// The field-by-field comparison, as the detector recorded it.
//
// Rows the client cannot name are skipped, never printed: a reader asked to
// compare `full_name` is being shown the database rather than their own
// records.
function EvidenceRows({ rows }: Readonly<{ rows: readonly Evidence[] }>) {
  const t = useT();
  const named = rows.filter((row) => FIELD_LABELS.has(row.field));
  if (named.length === 0) {
    return null;
  }
  return (
    <dl className="merge-evidence">
      {named.map((row) => {
        const fieldKey = FIELD_LABELS.get(row.field);
        const signalKey = SIGNAL_LABELS.get(row.signal);
        return (
          <div className="merge-evidence-row" key={row.field}>
            <dt className="t-caption merge-evidence-field">
              {fieldKey ? t(fieldKey) : row.field}
            </dt>
            <dd className="t-caption merge-evidence-values">
              <span>{row.left_value ?? t("day.merge.blank")}</span>
              <span className="merge-evidence-signal" data-signal={row.signal}>
                {signalKey ? t(signalKey) : row.signal}
              </span>
              <span>{row.right_value ?? t("day.merge.blank")}</span>
            </dd>
          </div>
        );
      })}
    </dl>
  );
}

// One duplicate, decided in place.
//
// `onMerge` carries the winner because the server needs to be told which record
// survives; `onKeepBoth` carries nothing because "these are different" is one
// fact with no parameter. Merge is the destructive half and is deliberately the
// one that needs a choice first — a reader cannot merge by reflex without
// having said which record they meant to keep.
export function MergeDecision({
  item,
  onMerge,
  onKeepBoth,
  pending,
  notice,
}: Readonly<{
  item: AttentionItem;
  onMerge: (winnerId: string) => void;
  onKeepBoth: () => void;
  pending: boolean;
  /** What a refused decision said, in the server's own words. */
  notice?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const pair = item.pair;
  const [winner, setWinner] = useState<string | null>(null);
  if (!pair) {
    // The reader may not see one of the two records, so the server sent no
    // pair. Naming that is the honest answer — a merge button over a record
    // somebody cannot read is worse than no button.
    return <p className="t-body merge-withheld">{t("day.merge.withheld")}</p>;
  }
  const percent = Math.round((item.confidence ?? 0) * 100);
  return (
    <div className="merge">
      <div className="merge-head">
        <p className="t-h3 merge-question">{t("day.merge.question")}</p>
        <Badge>
          {t("day.match", { percent: formatNumber(percent, locale) })}
        </Badge>
      </div>
      <div className="merge-sides">
        <SideChoice
          side={pair.left}
          name={`winner-${item.id}`}
          checked={winner === pair.left.id}
          onPick={() => setWinner(pair.left.id)}
          disabled={pending}
        />
        <SideChoice
          side={pair.right}
          name={`winner-${item.id}`}
          checked={winner === pair.right.id}
          onPick={() => setWinner(pair.right.id)}
          disabled={pending}
        />
      </div>
      <EvidenceRows rows={pair.evidence} />
      {notice && <Callout tone="danger">{notice}</Callout>}
      <div className="merge-verbs">
        {/* Primary only once it can actually be pressed.
            A filled button that refuses every click reads as a broken page
            rather than an unmet condition, and it was outweighing the verb
            beside it while doing nothing at all. Unfilled until a winner is
            chosen, it invites the choice instead of the click. */}
        <Button
          variant={winner ? "primary" : undefined}
          onClick={() => winner && onMerge(winner)}
          disabled={pending}
          reason={winner ? undefined : t("day.merge.pickFirst")}
        >
          {t("day.merge.cta")}
        </Button>
        <Button variant="ghost" onClick={onKeepBoth} disabled={pending}>
          {t("day.merge.keepBoth")}
        </Button>
      </div>
    </div>
  );
}
