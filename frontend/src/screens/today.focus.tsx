import { CheckCircle2 } from "lucide-react";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { problemMessageOf } from "./common";
import { MergeDecision } from "./today.merge";
import {
  type AttentionItem,
  attentionKey,
  useApproval,
  useDisposeDuplicate,
} from "./today.queries";
import "./today.focus.css";

// The decision lane, one decision at a time.
//
// A list of decisions asks a reader to choose which to answer before answering
// any, and every row has to earn its place by being scannable — which is why
// the list this replaces could only show a headline and a percentage, and why
// its every verb was a link to somewhere the decision actually lived.
//
// One at a time inverts that. The card can be as tall as the decision needs,
// because there is only one; the reader answers and the next arrives; the count
// goes down as they work. That is what makes the queue finishable, and it is
// the only arrangement where the number beside the lane and the thing on screen
// are talking about the same work.

// How far through the queue the reader is. Deliberately "3 of 90" rather than a
// bare remaining count: a number that only goes down says nothing about how
// much was there, and finishing is the point of the surface.
function Progress({ done, total }: Readonly<{ done: number; total: number }>) {
  const t = useT();
  const { locale } = useLocale();
  // Both figures are MAGNITUDES — how many decisions there are, and how far in
  // the reader is — so both are written in the reader's own notation. A German
  // reader reads "1.234"; the coerced form says "1234", which is a different
  // number to them.
  return (
    <p className="t-caption focus-progress">
      {t("day.focus.progress", {
        position: formatNumber(Math.min(done + 1, total), locale),
        total: formatNumber(total, locale),
      })}
    </p>
  );
}

// The cleared plate.
//
// A queue that ends in nothing looks broken, and a queue whose end is never
// drawn teaches a reader that it has no end. This is the state the whole
// surface is built to reach, so it is drawn deliberately rather than left as an
// absence.
function AllClear({ decided }: Readonly<{ decided: number }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="focus-clear">
      <CheckCircle2 size={32} aria-hidden className="focus-clear-mark" />
      <p className="t-h3 focus-clear-line">{t("day.focus.clear")}</p>
      {decided > 0 && (
        <p className="t-caption focus-clear-note">
          {t("day.focus.clearedCount", {
            count: formatNumber(decided, locale),
          })}
        </p>
      )}
    </div>
  );
}

// One item's body, chosen by what the item IS.
//
// A duplicate is a choice between two records; a staged proposal is an accept
// or a reject on one payload. They are different decisions and they get
// different bodies — the failure this replaces was a single generic row that
// could express neither, so both ended up as a link.
function FocusBody({
  item,
  onDone,
}: Readonly<{ item: AttentionItem; onDone: () => void }>) {
  const t = useT();
  const dispose = useDisposeDuplicate();
  const pending = dispose.isPending;

  if (item.source === "dedupe_candidate") {
    return (
      <MergeDecision
        item={item}
        pending={pending}
        // A refused merge must SAY so. The server refuses for reasons a reader
        // can act on — the workspace's own company cannot be merged into
        // another — and swallowing that leaves a button that appears to do
        // nothing, which is the failure this whole surface was built to end.
        notice={
          dispose.isError
            ? problemMessageOf(dispose.error, t, t("day.merge.refused"))
            : null
        }
        onMerge={(winnerId) =>
          dispose.mutate(
            { id: item.id, disposition: "merge", winner_id: winnerId },
            { onSuccess: onDone },
          )
        }
        onKeepBoth={() =>
          dispose.mutate(
            { id: item.id, disposition: "not_a_duplicate" },
            { onSuccess: onDone },
          )
        }
      />
    );
  }

  // Everything else on this lane is a staged proposal, and the row that decides
  // one already exists — the same component the record surfaces draw, carrying
  // edit-then-approve and reject-with-reason. A second spelling of a decision
  // is how one product comes to answer the same question two ways.
  //
  // The feed sends what a LANE needs — a sentence, a kind, a deadline — and the
  // row needs the whole proposal: the payload it edits, who staged it, what
  // evidence stands behind it. So the card fetches the one approval it is
  // showing. One card is on screen, so this is one read, and it is the same
  // read the record page makes.
  return <StagedDecision id={item.id} onDone={onDone} />;
}

// One staged proposal, fetched whole because it is the one being decided.
function StagedDecision({
  id,
  onDone,
}: Readonly<{ id: string; onDone: () => void }>) {
  const t = useT();
  const approval = useApproval(id);
  // A body that carries no `kind` is not a proposal this card can draw: the
  // kind chooses the label, the tool chip and the autonomy dot, and the row
  // reads it without asking. Treated as a FAILED read rather than rendered,
  // because the alternative is a render throw that takes the whole day's
  // surface down over one malformed answer — and `failed` is the honest word
  // for a read that came back unusable.
  const usable = approval.data?.kind ? approval.data : undefined;
  return (
    <SurfaceState
      label={t("day.needsYou")}
      labelLevel="h4"
      state={
        approval.isPending
          ? "loading"
          : approval.isError || (approval.data && !usable)
            ? "failed"
            : "ready"
      }
      loadingLabel={t("day.loading")}
      emptyLabel={t("day.needsYou.empty")}
      detail={{ onRetry: () => void approval.refetch() }}
    >
      {usable && (
        <ApprovalRow
          approval={usable}
          // The queue advances by REFETCHING, not by being told: a decided
          // proposal is no longer pending, so the next read simply does not
          // carry it and the card behind it becomes the card in front.
          //
          // `onAlreadyDecided` is not that signal and must not be used as one —
          // it fires only when the server refuses because somebody else decided
          // this first. Wiring the advance to it would leave the lane frozen on
          // every proposal the reader answers normally.
          onAlreadyDecided={onDone}
          extraInvalidateKeys={[attentionKey]}
        />
      )}
    </SurfaceState>
  );
}

// The lane: one card, its position in the queue, and the way past it.
export function FocusLane({
  items,
  total,
  decided,
  withheld,
  onDecided,
  onSkip,
}: Readonly<{
  items: readonly AttentionItem[];
  /** What the QUEUE holds, which is not what this page carries. */
  total: number;
  /** How many the reader has answered in this sitting. */
  decided: number;
  withheld: boolean;
  onDecided: () => void;
  onSkip: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const current = items[0];
  // The lane owns the toast, not the card. Answering a proposal replaces the
  // card — that is how the queue advances — and a sticky offer owned by the
  // card would leave with it, which is the one message that must survive the
  // card it was shown from.
  return (
    <Panel
      title={
        <span className="focus-title">
          {t("day.needsYou")}
          {total > 0 && <Badge>{formatNumber(total, locale)}</Badge>}
        </span>
      }
      tone="accent"
    >
      <PanelBody>
        {withheld ? (
          <p className="t-body focus-withheld">{t("day.lane.withheld")}</p>
        ) : current ? (
          <div className="focus">
            <Progress done={decided} total={total} />
            <FocusBody item={current} onDone={onDecided} />
            {/* Later, not never. A queue whose only exits are terminal is one
                a reader stops opening the moment it holds something they
                cannot answer yet. */}
            <div className="focus-defer">
              <Button small onClick={onSkip}>
                {t("day.focus.later")}
              </Button>
            </div>
          </div>
        ) : (
          <AllClear decided={decided} />
        )}
      </PanelBody>
    </Panel>
  );
}
