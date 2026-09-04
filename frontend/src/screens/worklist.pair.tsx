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

import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemCodeOf } from "./common";
import { useDedupeDisposition } from "./dedupe.queries";
import { type WorklistItem, worklistKey } from "./worklist.queries";

/**
 * The pair, and the verbs that answer it.
 *
 * Drawn only where the server sent the payload. A row that arrives without it
 * is one whose reader may not see both sides, and the lane already withheld the
 * `merge` verb for exactly that reason — so there is nothing here to draw and
 * no decision to offer.
 *
 * The verbs are drawn only where the server OFFERED them. Settling a pair
 * archives one record and rewrites the other, so it belongs to whoever could
 * change both — the owner, or a workspace-wide seat. A reader without that
 * authority still sees the pair, because knowing a duplicate is waiting is not
 * the same as being able to settle it, and is told who can. Rendering the
 * buttons for everybody is what produced a control that refused every press a
 * rep made on a colleague's records, then advised trying again.
 */
export function PairDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const decide = useDedupeDisposition([worklistKey]);
  const pair = item.pair;
  if (!pair) {
    return null;
  }
  const mayDecide = item.actions?.includes("merge") ?? false;
  const answer = (
    disposition: "merge" | "not_a_duplicate",
    winnerId?: string,
  ) =>
    decide.mutate(
      { id: item.id, disposition, winnerId },
      {
        // A refused decision leaves the row exactly as an unpressed one looks.
        // Without this the reader believes the pair is settled and it is not.
        //
        // THREE outcomes, not one. A refusal will refuse again however many
        // times it is pressed, so "try again" is the wrong instruction — the
        // reader needs a steward. A conflict means somebody settled the pair
        // first, and the row is already gone. Only the rest is worth a retry.
        onError: (error) => {
          const code = problemCodeOf(error);
          const message =
            code === "permission_denied"
              ? "worklist.pair.refused"
              : code === "conflict"
                ? "worklist.pair.alreadySettled"
                : "worklist.pair.failed";
          toast.show(t(message), { mark: false });
        },
      },
    );
  return (
    <div className="worklist-pair">
      <p className="t-caption worklist-pair-ask">{t("worklist.pair.ask")}</p>
      <ul className="worklist-pair-sides">
        {[pair.left, pair.right].map((side) => (
          <li key={side.id} className="worklist-pair-side">
            <span className="worklist-pair-name">{side.label}</span>
            {side.detail && (
              <span className="t-caption worklist-pair-detail">
                {side.detail}
              </span>
            )}
            {/* The reader's best single signal for which side is the real
                one, where the record type carries such a count. */}
            {side.related_count !== undefined && (
              <span className="t-caption worklist-pair-related">
                {t("worklist.pair.related", {
                  count: formatNumber(side.related_count, locale),
                })}
              </span>
            )}
            {/* The RECORD'S OWN NAME in the verb, not "Keep this one" twice.
                Two identically named buttons over an irreversible merge leave
                a reader who is not looking at the layout — a screen reader,
                a keyboard walking the controls — no way to tell which record
                they are about to archive. */}
            {mayDecide && (
              <Button
                small
                variant="primary"
                pending={decide.isPending}
                onClick={() => answer("merge", side.id)}
              >
                {t("worklist.pair.keep", { name: side.label })}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {mayDecide ? (
        <Button
          small
          pending={decide.isPending}
          onClick={() => answer("not_a_duplicate")}
        >
          {t("worklist.pair.notDuplicate")}
        </Button>
      ) : (
        // Said in words rather than shown as disabled buttons. A greyed-out
        // control asks the reader to work out why it is grey; a sentence tells
        // them the pair is real, that they cannot settle it, and who can.
        <p className="t-caption worklist-pair-steward">
          {t("worklist.pair.stewardOnly")}
        </p>
      )}
    </div>
  );
}
