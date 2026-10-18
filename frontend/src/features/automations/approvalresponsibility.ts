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
): readonly ResolvedDisplayField[] {
  const promiser = change.promiser_seat_id;
  const assignee = change.seat_id;
  const party = typeof change.party === "string" ? change.party.trim() : "";
  const madeBy =
    typeof promiser === "string" && promiser
      ? (names.get(promiser) ?? t("approval.commitment.unnamedColleague"))
      : party || t("approval.commitment.unresolved");
  const effect =
    typeof assignee === "string" && assignee
      ? t("approval.commitment.assignNamed", {
          name:
            names.get(assignee) ?? t("approval.commitment.unnamedColleague"),
        })
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
