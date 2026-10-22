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
    const change =
      approval.kind === "commitment_task"
        ? (approval.proposed_change ?? {})
        : {};
    return [
      approval.requested_decider_id,
      change.promiser_seat_id,
      change.seat_id,
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
  // A new task with no seat of its own goes to the colleague who made the
  // promise, as the server assigns it.
  const assignee =
    typeof change.seat_id === "string" && change.seat_id
      ? change.seat_id
      : promiser;
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
