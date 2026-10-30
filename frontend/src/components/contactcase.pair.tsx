// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../api/schema";
import { ActionRow } from "../design-system/actionrow";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type PairDecisionFailure,
  pairDecisionFailure,
  useDedupeDisposition,
  useDedupeUndo,
} from "../lib/dedupe.queries";
import type { ContactCase, ContactCaseMember } from "./contactcase.queries";

type Edge = components["schemas"]["ContactCaseEdge"];

const FAILURE_MESSAGES = {
  refused: "contactCase.pair.refused",
  failed: "contactCase.pair.failed",
} as const satisfies Record<
  Exclude<PairDecisionFailure, "settled">,
  MessageKey
>;

/** Whether this reader may merge this pair here: only a too-large case asks pair by pair. */
function mayMergePair(contactCase: ContactCase, edge: Edge): boolean {
  return contactCase.too_large && edge.can_merge;
}

/**
 * Whether this reader is offered any verb on one pair, as the server answers
 * it per pair. Dismissing and merging are asked for apart, because merging
 * two needs more authority than saying they differ.
 */
export function mayDecidePair(contactCase: ContactCase, edge: Edge): boolean {
  return edge.can_dismiss || mayMergePair(contactCase, edge);
}

function memberOf(contactCase: ContactCase, id: string) {
  return contactCase.members.find((member) => member.id === id);
}

// Two members often share a name, since that is what made them look alike.
// The email then joins the spoken name, so the two Keep buttons differ.
function keepLabel(
  side: ContactCaseMember | undefined,
  other: ContactCaseMember | undefined,
  t: ReturnType<typeof useT>,
): string {
  const visible = t("contactCase.pair.keep", { name: side?.name ?? "" });
  if (side?.name !== other?.name || !side?.email) return visible;
  return `${visible} — ${side.email}`;
}

/** The verbs on one pair of the case: dismiss, merge the two, or undo. */
export function PairActions({
  contactCase,
  edge,
  onStale,
  onMerged,
}: Readonly<{
  contactCase: ContactCase;
  edge: Edge;
  onStale: () => void;
  onMerged: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const decide = useDedupeDisposition();
  const undo = useDedupeUndo();
  const [choosing, setChoosing] = useState(false);
  const left = memberOf(contactCase, edge.left_id);
  const right = memberOf(contactCase, edge.right_id);
  const pairName = t("contactCase.evidence.pair", {
    left: left?.name ?? "",
    right: right?.name ?? "",
  });
  const pending = decide.isPending || undo.isPending;
  // A settled pair was decided by someone else first. The review reads the
  // case again and says so, instead of a toast over a stale list.
  const onError = (error: Error) => {
    const failure = pairDecisionFailure(error);
    if (failure === "settled") {
      onStale();
      return;
    }
    toast.show(t(FAILURE_MESSAGES[failure]), {
      tone: "danger",
      sticky: failure === "refused",
    });
  };

  if (edge.status === "not_a_duplicate") {
    if (!edge.can_dismiss) return null;
    return (
      <ActionRow>
        <Button
          pending={undo.isPending}
          aria-label={`${t("common.undo")}: ${pairName}`}
          onClick={() => undo.mutate({ id: edge.pair_id }, { onError })}
        >
          {t("common.undo")}
        </Button>
      </ActionRow>
    );
  }
  if (choosing) {
    return (
      <div className="contactcase-section">
        <p className="t-caption">{t("contactCase.preview.final")}</p>
        <ActionRow>
          {/* With write access to one end only, the server names the end that
              survives, and only that one may be kept. */}
          {[left, right]
            .filter((side) => !edge.keep_id || side?.id === edge.keep_id)
            .map((side) => (
              <Button
                key={side?.id}
                variant="danger"
                pending={decide.isPending}
                aria-label={keepLabel(side, side === left ? right : left, t)}
                onClick={() =>
                  decide.mutate(
                    {
                      id: edge.pair_id,
                      disposition: "merge",
                      winnerId: side?.id,
                      contactIds: [edge.left_id, edge.right_id],
                    },
                    { onError, onSuccess: onMerged },
                  )
                }
              >
                {t("contactCase.pair.keep", { name: side?.name ?? "" })}
              </Button>
            ))}
          <Button disabled={pending} onClick={() => setChoosing(false)}>
            {t("create.cancel")}
          </Button>
        </ActionRow>
      </div>
    );
  }
  return (
    <ActionRow
      primary={
        mayMergePair(contactCase, edge) ? (
          <Button
            disabled={pending}
            aria-label={`${t("contactCase.pair.mergeTwo")}: ${pairName}`}
            onClick={() => setChoosing(true)}
          >
            {t("contactCase.pair.mergeTwo")}
          </Button>
        ) : undefined
      }
    >
      {edge.can_dismiss && (
        <Button
          pending={decide.isPending}
          aria-label={`${t("contactCase.pair.notSame")}: ${pairName}`}
          onClick={() =>
            decide.mutate(
              { id: edge.pair_id, disposition: "not_a_duplicate" },
              { onError },
            )
          }
        >
          {t("contactCase.pair.notSame")}
        </Button>
      )}
    </ActionRow>
  );
}
