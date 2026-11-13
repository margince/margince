import type { Translator } from "../../i18n";
import { useMemberNames } from "../../lib/membernames";
import type { ResolvedDisplayField } from "./approvaldisplay";

type Proposal = Readonly<{
  kind: string;
  requested_decider_id?: string | null;
  proposed_change?: Readonly<Record<string, unknown>> | null;
}>;

export function useApprovalNames(approvals: readonly Proposal[]) {
  const ids = approvals.flatMap((approval) => {
    const change = approval.proposed_change ?? {};
    const commitment = approval.kind === "commitment_task";
    return [
      approval.requested_decider_id,
      commitment ? change.promiser_seat_id : undefined,
      commitment ? change.seat_id : undefined,
      chosenDoer(approval.kind, change),
    ].filter((id): id is string => typeof id === "string" && id.length > 0);
  });
  return useMemberNames(ids);
}

export function commitmentResponsibility(
  change: Readonly<Record<string, unknown>>,
  names: ReadonlyMap<string, string>,
  t: Translator,
  assigneeInactive = false,
): readonly ResolvedDisplayField[] {
  const promiser = change.promiser_seat_id;
  // The colleague the accepter chose, else the card's own seat, else the
  // colleague who made the promise, as the server assigns it.
  const assignee =
    chosenDoer("commitment_task", change) ??
    (typeof change.seat_id === "string" && change.seat_id
      ? change.seat_id
      : promiser);
  const party = typeof change.party === "string" ? change.party.trim() : "";
  const madeBy =
    typeof promiser === "string" && promiser
      ? (names.get(promiser) ?? party) ||
        t("approval.commitment.unnamedColleague")
      : change.promiser_unresolved === true && party
        ? t("approval.commitment.unverifiedNamed", { name: party })
        : party || t("approval.commitment.unresolved");
  const assigneeName =
    (typeof assignee === "string" && names.get(assignee)) ||
    t("approval.commitment.unnamedColleague");
  const effect = assigneeInactive
    ? t("approval.commitment.inactiveAssignment", { name: assigneeName })
    : t("approval.commitment.assignNamed", { name: assigneeName });
  return [
    { field: "party", label: t("approval.commitment.promiser"), value: madeBy },
    {
      field: "assignment",
      label: t("approval.commitment.effect"),
      value: effect,
    },
  ];
}

/**
 * The "Promised by" row a commitment's preview needs beside the owner it
 * names, because the owner row alone does not say whose word it was. A
 * request to add recipients names no new promise, so it shows none.
 */
export function madeByRow(
  kind: string,
  change: Readonly<Record<string, unknown>>,
  names: ReadonlyMap<string, string>,
  t: Translator,
): readonly ResolvedDisplayField[] {
  if (kind !== "commitment_task" || typeof change.existing_task_id === "string")
    return [];
  return commitmentResponsibility(change, names, t).filter(
    (entry) => entry.field === "party",
  );
}

export function approvalAcceptable(
  approval: Readonly<{
    assignee_inactive?: boolean;
    preview?: {
      version: number;
      state: string;
      conflicts?: readonly { blocks_approval: boolean }[];
    };
  }>,
): boolean {
  return (
    approval.assignee_inactive !== true &&
    (!approval.preview ||
      ((approval.preview.version === 1 || approval.preview.version === 2) &&
        approval.preview.state === "ready" &&
        !approval.preview.conflicts?.some(
          (conflict) => conflict.blocks_approval,
        )))
  );
}

/** The cards whose accepter may choose who does the work (`doer_id`). */
const DOER_KINDS: readonly string[] = ["commitment_task", "deal_follow_up"];

/**
 * The colleague chosen to do a card's work, read from its `doer_id`: set when
 * the accepter picked somebody, so a decided card names them. Undefined while
 * the card keeps its own rule.
 */
export function chosenDoer(
  kind: string,
  change: Readonly<Record<string, unknown>>,
): string | undefined {
  const doer = change.doer_id;
  return DOER_KINDS.includes(kind) && typeof doer === "string" && doer
    ? doer
    : undefined;
}

/** A follow-up card's "Who does this" row, once a colleague was chosen. */
export function followUpDoerRow(
  change: Readonly<Record<string, unknown>>,
  names: ReadonlyMap<string, string>,
  t: Translator,
): readonly ResolvedDisplayField[] {
  const doer = chosenDoer("deal_follow_up", change);
  if (!doer) return [];
  return [
    {
      field: "doer_id",
      label: t("approval.field.doer"),
      value: names.get(doer) ?? t("approval.commitment.unnamedColleague"),
    },
  ];
}
