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
  const assignee = change.seat_id;
  const party = typeof change.party === "string" ? change.party.trim() : "";
  const madeBy =
    typeof promiser === "string" && promiser
      ? (names.get(promiser) ?? party) ||
        t("approval.commitment.unnamedColleague")
      : change.promiser_unresolved === true && party
        ? t("approval.commitment.unverifiedNamed", { name: party })
        : party || t("approval.commitment.unresolved");
  const assigned = typeof assignee === "string" && assignee.length > 0;
  const assigneeName = assigned
    ? (names.get(assignee) ?? t("approval.commitment.unnamedColleague"))
    : t("approval.commitment.unnamedColleague");
  const effect = assigneeInactive
    ? t("approval.commitment.inactiveAssignment", { name: assigneeName })
    : assigned
      ? t("approval.commitment.assignNamed", { name: assigneeName })
      : t("approval.commitment.assignDecider");
  return [
    { field: "party", label: t("approval.commitment.promiser"), value: madeBy },
    {
      field: "assignment",
      label: t("approval.commitment.effect"),
      value: effect,
    },
  ];
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
      (approval.preview.version === 1 &&
        approval.preview.state === "ready" &&
        !approval.preview.conflicts?.some(
          (conflict) => conflict.blocks_approval,
        )))
  );
}
