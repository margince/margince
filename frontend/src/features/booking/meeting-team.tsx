// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { usePatchSchedulingProfile } from "../../lib/scheduling-profile-query";

type Profile = components["schemas"]["SchedulingProfile"];
type Member = components["schemas"]["SchedulingTeamMember"];
type Request = components["schemas"]["SchedulingTeamRequest"];

// The limit crm.yaml gives SchedulingProfile.team_members.
const MAX_MEMBERS = 5;

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

function memberStatus(member: Member): MemberStatus {
  return MEMBER_STATE[member.state ?? "pending"] ?? PENDING;
}

/**
 * The colleagues who attend every meeting booked through the host's link.
 * Guests see only times when all of them are free.
 */
export function TeamMembersCard({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const write = usePatchSchedulingProfile();
  const candidates = useQuery({
    queryKey: ["scheduling-team-candidates"],
    queryFn: async () => unwrap(await api.GET("/scheduling/team-candidates")),
  });
  const members = profile.team_members ?? [];
  const named = new Set(members.map((member) => member.user_id));
  const offered = (candidates.data ?? [])
    .filter((candidate) => !named.has(candidate.user_id))
    .map((candidate) => ({ value: candidate.user_id, label: candidate.name }));
  const full = members.length >= MAX_MEMBERS;
  const save = (next: readonly Member[]) =>
    write.mutate({
      team_members: next.map((member) => ({ user_id: member.user_id })),
    });
  return (
    <Panel
      title={t("scheduling.team.title")}
      footer={<span className="t-caption">{t("scheduling.team.foot")}</span>}
    >
      <PanelBody className="form-stack">
        {members.length > 0 && (
          <SettingList>
            {members.map((member) => {
              const status = memberStatus(member);
              return (
                <SettingRow
                  key={member.user_id}
                  label={member.name ?? ""}
                  description={
                    <StatusBadge kind={status.kind} label={t(status.key)} />
                  }
                  control={
                    <Button
                      pending={write.isPending}
                      onClick={() =>
                        save(
                          members.filter((m) => m.user_id !== member.user_id),
                        )
                      }
                    >
                      {t("scheduling.team.remove")}
                    </Button>
                  }
                />
              );
            })}
          </SettingList>
        )}
        <SettingList>
          <SettingRow
            label={t("scheduling.team.add")}
            description={t(
              full ? "scheduling.team.full" : "scheduling.team.addHelp",
            )}
            control={(control) => (
              <Select
                aria-labelledby={control["aria-labelledby"]}
                aria-describedby={control["aria-describedby"]}
                options={offered}
                value=""
                placeholder={t(
                  offered.length === 0 && candidates.isSuccess
                    ? "scheduling.team.noneToAdd"
                    : "scheduling.team.choose",
                )}
                disabled={full || offered.length === 0 || write.isPending}
                onChange={(user) => save([...members, { user_id: user }])}
              />
            )}
          />
        </SettingList>
        <ErrorLine error={write.error ?? candidates.error} />
      </PanelBody>
    </Panel>
  );
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
      host,
      state,
    }: Readonly<{ host: string; state: "confirmed" | "left" }>) => {
      unwrap(
        await api.PUT("/scheduling/team-requests/{host_user_id}", {
          params: { path: { host_user_id: host } },
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
              key={request.host_user_id}
              request={request}
              pending={answer.isPending}
              onAnswer={(state) =>
                answer.mutate({ host: request.host_user_id, state })
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
