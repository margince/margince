// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import type { StatusKind } from "../../design-system/statusbadge";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";

type Member = components["schemas"]["SchedulingTeamMember"];
type Request = components["schemas"]["SchedulingTeamRequest"];

type MemberStatus = Readonly<{ kind: StatusKind; key: MessageKey }>;

const PENDING: MemberStatus = {
  kind: "attention",
  key: "scheduling.team.pending",
};

// A state this build does not know reads as waiting, the safe reading.
const MEMBER_STATE: Readonly<Record<string, MemberStatus>> = {
  pending: PENDING,
  confirmed: { kind: "done", key: "scheduling.team.confirmed" },
  left: { kind: "failure", key: "scheduling.team.left" },
  unavailable: { kind: "failure", key: "scheduling.team.unavailable" },
};

export function memberStatus(member: Member): MemberStatus {
  return MEMBER_STATE[member.state ?? "pending"] ?? PENDING;
}

/** The colleagues' links that name this member, to confirm or leave. */
export function TeamRequestsCard() {
  const t = useT();
  const client = useQueryClient();
  const requests = useQuery({
    queryKey: ["scheduling-team-requests"],
    queryFn: async () => unwrap(await api.GET("/scheduling/team-requests")),
  });
  const answer = useMutation({
    mutationFn: async ({
      page,
      state,
    }: Readonly<{ page: string; state: "confirmed" | "left" }>) => {
      unwrap(
        await api.PUT("/scheduling/team-requests/{page_id}", {
          params: { path: { page_id: page } },
          body: { state },
        }),
      );
    },
    onSuccess: () =>
      client.invalidateQueries({ queryKey: ["scheduling-team-requests"] }),
  });
  const rows = requests.data ?? [];
  if (rows.length === 0) return null;
  return (
    <Panel title={t("scheduling.team.requestsTitle")}>
      <PanelBody className="form-stack">
        <SettingList>
          {rows.map((request) => (
            <RequestRow
              key={request.page_id}
              request={request}
              pending={answer.isPending}
              onAnswer={(state) =>
                answer.mutate({ page: request.page_id, state })
              }
            />
          ))}
        </SettingList>
        <ErrorLine error={answer.error} />
      </PanelBody>
    </Panel>
  );
}

function RequestRow({
  request,
  pending,
  onAnswer,
}: Readonly<{
  request: Request;
  pending: boolean;
  onAnswer: (state: "confirmed" | "left") => void;
}>) {
  const t = useT();
  const confirmed = request.state === "confirmed";
  return (
    <SettingRow
      label={t("scheduling.team.requestLabel", {
        name: request.host_name,
        title: request.title,
      })}
      description={t(
        confirmed
          ? "scheduling.team.requestConfirmed"
          : "scheduling.team.requestPending",
      )}
      control={
        <div className="cell-actions">
          {!confirmed && (
            <Button
              variant="primary"
              pending={pending}
              onClick={() => onAnswer("confirmed")}
            >
              {t("scheduling.team.confirm")}
            </Button>
          )}
          <Button pending={pending} onClick={() => onAnswer("left")}>
            {t("scheduling.team.leave")}
          </Button>
        </div>
      }
    />
  );
}
