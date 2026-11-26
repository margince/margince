// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Deciding a duplicate pair where it is shown.
//
// The contract has always sent both records on a `dedupe_candidate` row, and
// says why in as many words: "so the decision can be MADE where it is shown —
// a card that named neither record could only ask a reader to go and look,
// which is the hand-off this surface exists to remove." The Worklist showed
// the row, dropped the payload, and asked the reader to go and look.
//
// Two answers, and they are not symmetrical. Merging KEEPS one record and
// archives the other, so the reader has to say which survives — there is no
// sensible default, and picking one for them would be the product choosing
// which of a customer's two records is the real one. Saying they are not the
// same needs no such choice and settles the pair for everybody, for good.
//
// The asymmetry reaches the OFFER too, which is why the two verbs are guarded
// apart. Two companies each running live projects cannot be combined by
// anybody — nothing in the data says which body of work is which — and the
// lane withholds `merge` on a pair like that however much authority the reader
// holds. The dismissal is still theirs: a false positive is exactly what this
// pair is, and clearing it changes neither record. Guarding both on `merge` is
// what left such a pair on the page as a question nobody could answer here.

import { type ReactNode, useState } from "react";
import { ContactCaseReview } from "../../components/contactcase";
import { EvidenceLine } from "../../components/contactcase.evidence";
import { type MergeSide, mergeSentence } from "../../components/merge.sentence";
import { AppLink } from "../../design-system/applink";
import { Button, Card } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { undoAction, useToast } from "../../design-system/toast";
import { formatDate, formatDateTime, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type PairDecisionFailure,
  type PairDisposition,
  pairDecisionFailure,
  useDedupeDisposition,
  useDedupeUndo,
} from "../../lib/dedupe.queries";
import { recordRoute } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
// The review's own sheet: the two columns, the recessed cards and the trailing
// line of verbs. It travels with the component because two hosts draw it.
import "./worklist.pair.css";
import type { WorklistItem } from "./worklist.queries";

type PairSide = NonNullable<WorklistItem["pair"]>["left"];
type Describe = (iso: string) => string;

// Two same-name records are told apart by the first of these that differs:
// detail or creation day, creation minute, ID tail, whole ID. A step
// is taken only while the two still read alike. `step` names the one that told.
type Told = Readonly<{
  lines: readonly [string | undefined, string | undefined];
  step: number;
}>;

const MINUTE_STEP = 1;
const ID_STEP = 2;

function sideDetails(
  pair: NonNullable<WorklistItem["pair"]>,
  created: Readonly<{ day: Describe; minute: Describe }>,
): Told {
  const { left, right } = pair;
  if (left.label !== right.label) {
    return { lines: [left.detail, right.detail], step: 0 };
  }
  const steps: ((side: PairSide) => string | undefined)[] = [
    (side) =>
      side.detail ||
      (side.created_at ? created.day(side.created_at) : undefined),
    (side) => (side.created_at ? created.minute(side.created_at) : undefined),
    (side) => side.id.slice(-6),
    (side) => side.id,
  ];
  let told: Told = { lines: [undefined, undefined], step: 0 };
  for (const [step, describe] of steps.entries()) {
    told = { lines: [describe(left), describe(right)], step };
    if (told.lines[0] !== told.lines[1]) {
      break;
    }
  }
  return told;
}

// keepLabel is what the Keep button is CALLED, which is not always what it says.
//
// Where the two names collide, the side's distinguishing line joins the
// accessible name, so two buttons over an irreversible merge never share one.
// The visible text stays its prefix, so the spoken and seen names agree.
function keepLabel(
  pair: NonNullable<WorklistItem["pair"]>,
  side: MergeSide,
  t: ReturnType<typeof useT>,
): string {
  const visible = t("worklist.pair.keep", { name: side.name });
  if (pair.left.label !== pair.right.label || !side.detail) {
    return visible;
  }
  return `${visible} — ${side.detail}`;
}

// The two names are the cards' own titles, so a name match repeats them.
const TITLE_FIELDS: ReadonlySet<string> = new Set([
  "display_name",
  "full_name",
]);

const FAILURE_MESSAGES = {
  refused: "worklist.pair.refused",
  settled: "worklist.pair.alreadySettled",
  failed: "worklist.pair.failed",
} as const satisfies Record<PairDecisionFailure, MessageKey>;

/**
 * The pair, and the verbs that answer it.
 *
 * A row without the payload is one whose reader may not see both sides. The
 * lane withheld its verbs for that reason, so nothing is drawn here.
 *
 * Each verb is drawn only where the server offered it. Deciding a pair archives
 * or rewrites records, so it belongs to whoever could change both: the owner or
 * a workspace-wide seat. The merge also needs a pair it would take. A reader
 * without that authority still sees the pair, and is told who can settle it.
 */
export function PairDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const toast = useToast();
  const decide = useDedupeDisposition();
  const undo = useDedupeUndo();
  const [reviewing, setReviewing] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);
  const pair = item.pair;
  if (!pair) {
    return null;
  }
  const mayMerge = item.actions?.includes("merge") ?? false;
  // Two contacts merge only through the review, where the reader ticks both
  // and sees the preview. Keep opens it on the contact they chose to keep.
  const reviewed = item.kind === "contact";
  const mayDismiss = item.actions?.includes("dismiss") ?? false;
  const created = (iso: string) =>
    t("worklist.pair.created", { date: formatDate(iso, locale, zone) });
  const told = sideDetails(pair, {
    day: created,
    minute: (iso) =>
      t("worklist.pair.created", { date: formatDateTime(iso, locale, zone) }),
  });
  const toldOf = (side: PairSide) =>
    told.lines[side.id === pair.left.id ? 0 : 1];
  const named = (side: PairSide): MergeSide => ({
    name: side.label,
    detail: toldOf(side),
  });
  // The kept side and the side a merge into it archives.
  const merging = (keptId: string) =>
    keptId === pair.left.id
      ? { kept: named(pair.left), archived: named(pair.right) }
      : { kept: named(pair.right), archived: named(pair.left) };
  // A refused decision leaves the row as an unpressed one looks.
  // Without this the reader believes the pair is settled and it is not.
  const refused = (error: unknown) => {
    const failure = pairDecisionFailure(error);
    // The two REASONED refusals stay until the reader dismisses them; they say
    // something to act on elsewhere, and only "Retry" is worth a second press.
    toast.show(t(FAILURE_MESSAGES[failure]), {
      tone: "danger",
      sticky: failure !== "failed",
    });
  };
  const answer = (
    disposition: PairDisposition,
    winnerId: string | undefined,
    settled: () => void,
  ) =>
    // The promise rather than mutate's own callbacks: a settled pair leaves
    // the list, and the row's unmount would swallow the message saying so.
    decide
      .mutateAsync({
        id: item.id,
        disposition,
        winnerId,
        companyIds:
          item.kind === "company" ? [pair.left.id, pair.right.id] : undefined,
      })
      .then(settled, refused);
  const merge = (keptId: string) => {
    const { kept, archived } = merging(keptId);
    answer("merge", keptId, () => {
      setConfirming(null);
      toast.show(mergeSentence(t, "merge.done", archived, kept));
    });
  };
  // Saying they differ changes neither record, so it takes no question and
  // has one clean way back.
  const dismiss = () =>
    answer("not_a_duplicate", undefined, () =>
      toast.show(t("worklist.pair.dismissed"), {
        action: undoAction(t("common.undo"), () =>
          undo.mutateAsync({ id: item.id }).catch(refused),
        ),
      }),
    );
  const confirmed = confirming ? merging(confirming) : null;
  const evidence = pair.evidence.filter(
    (line) => !TITLE_FIELDS.has(line.field),
  );
  return (
    <div className="worklist-pair-review">
      {reviewing && (
        <ContactCaseReview
          contactId={reviewing}
          open
          onClose={() => setReviewing(null)}
        />
      )}
      {/* A merge archives a record for good, so it is asked as the record
          menu's Merge asks it, naming both sides. */}
      {confirming && confirmed && (
        <ConfirmModal
          open
          onClose={() => setConfirming(null)}
          title={t("worklist.pair.keep", { name: confirmed.kept.name })}
          confirmLabel={t("merge.submit")}
          confirmVariant="danger"
          pending={decide.isPending}
          onConfirm={() => merge(confirming)}
        >
          <p>
            {mergeSentence(
              t,
              "merge.confirm",
              confirmed.archived,
              confirmed.kept,
            )}
          </p>
        </ConfirmModal>
      )}
      {/* THE QUESTION, as the block's lead line. What follows it is the two
          records it is about and the one answer that is about neither.

          Which question it is depends on what can be done. "Which record
          should survive?" over a card with no Keep button asks for an answer
          the page will not take; where the merge is off the table, what is
          actually being asked is whether these two are the same at all. */}
      <p className="t-body worklist-pair-ask">
        {t(
          mayDismiss && !mayMerge
            ? "worklist.pair.mergeBlocked"
            : "worklist.pair.ask",
        )}
      </p>
      {evidence.length > 0 && (
        <ul className="worklist-pair-evidence">
          {evidence.map((line) => (
            <EvidenceLine
              key={`${line.field}-${line.signal}`}
              evidence={line}
            />
          ))}
        </ul>
      )}
      <ul className="worklist-pair-cards">
        {[pair.left, pair.right].map((side) => (
          <PairSideCard
            key={side.id}
            side={side}
            kind={item.kind}
            created={created}
            told={told.step >= MINUTE_STEP ? toldOf(side) : undefined}
            byId={told.step >= ID_STEP}
          >
            {/* At the card's own FOOT, on its trailing edge: the verb belongs
                to the record above it. It NAMES that record, so a reader who
                is not looking at the layout knows which one they keep. */}
            {mayMerge && (!pair.keep_id || pair.keep_id === side.id) && (
              <div className="card-actions">
                <Button
                  variant="primary"
                  pending={decide.isPending}
                  aria-label={keepLabel(pair, named(side), t)}
                  onClick={() =>
                    reviewed ? setReviewing(side.id) : setConfirming(side.id)
                  }
                >
                  {t("worklist.pair.keep", { name: side.label })}
                </Button>
              </div>
            )}
          </PairSideCard>
        ))}
      </ul>
      {mayDismiss && (
        // ONE LINE UNDER BOTH CARDS, on the trailing edge every other line of
        // verbs in the queue stands on, and nothing on it is filled: the two
        // Keep verbs are the answers to the question, and a fill here would
        // offer "not the same" as the expected one of three.
        //
        // Where the merge is withheld this is the only verb left, and it is
        // still ghost: the pair being unmergeable is not a reason to press
        // harder for the answer that says it was never a pair.
        <div className="worklist-pair-actions">
          <Button variant="ghost" pending={decide.isPending} onClick={dismiss}>
            {t("worklist.pair.notDuplicate")}
          </Button>
        </div>
      )}
      {!mayMerge && !mayDismiss && (
        // Said in words rather than shown as disabled buttons. A greyed-out
        // control asks the reader to work out why it is grey; a sentence tells
        // them the pair is real, that they cannot settle it, and who can.
        <p className="worklist-pair-steward">
          {t("worklist.pair.stewardOnly")}
        </p>
      )}
    </div>
  );
}

// One candidate, on the catalog's recessed card: `Card inset` rather than
// `PanelPlate`, which holds context and never a control. A list item, so the
// two stay one list of two.
function PairSideCard({
  side,
  kind,
  created,
  told,
  byId,
  children,
}: Readonly<{
  side: PairSide;
  kind: string | undefined;
  created: Describe;
  // Shown where the creation day alone reads the same on both cards: the
  // creation minute in its place, or the ID beside it.
  told: string | undefined;
  byId: boolean;
  children: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const route = recordRoute(kind, side.id);
  return (
    <Card as="li" inset className="worklist-pair-card">
      {route ? (
        <AppLink href={routeHash(route)}>{side.label}</AppLink>
      ) : (
        <span>{side.label}</span>
      )}
      {side.detail && <span className="t-caption">{side.detail}</span>}
      {told && !byId ? (
        <span className="t-caption">{told}</span>
      ) : (
        side.created_at && (
          <span className="t-caption">{created(side.created_at)}</span>
        )
      )}
      {told && byId && <span className="t-caption">{told}</span>}
      {/* The reader's best single signal for which side is the real one,
          where the record type carries such a count. */}
      {side.related_count !== undefined && (
        <span className="t-caption">
          {t("worklist.pair.related", {
            count: formatNumber(side.related_count, locale),
          })}
        </span>
      )}
      {children}
    </Card>
  );
}
