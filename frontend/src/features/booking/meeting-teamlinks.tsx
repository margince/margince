// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { listFormat } from "../../format/formatters";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { memberStatus } from "./meeting-team";

type Link = components["schemas"]["SchedulingTeamLink"];
type Member = components["schemas"]["SchedulingTeamMember"];
type Candidate = components["schemas"]["SchedulingTeamCandidate"];
type Locale = ReturnType<typeof useLocale>["locale"];

// The limit crm.yaml gives SchedulingTeamLinkRequest.team_members.
const MAX_MEMBERS = 5;

const LINKS_KEY = ["scheduling-team-links"] as const;

type LinkStatus = Readonly<{ kind: StatusKind; key: MessageKey }>;

// A blocker this build does not know reads as paused, the safe reading.
const LINK_STATUS: Readonly<Record<string, LinkStatus>> = {
  none: { kind: "done", key: "scheduling.teamLinks.ready" },
  team_not_ready: { kind: "attention", key: "scheduling.teamLinks.waiting" },
};
const PAUSED: LinkStatus = {
  kind: "failure",
  key: "scheduling.teamLinks.paused",
};
const OFF: LinkStatus = { kind: "neutral", key: "scheduling.teamLinks.off" };

/** The default name of a team link: the host and each colleague on it. */
export function teamLinkTitle(
  t: Translator,
  locale: Locale,
  host: string,
  names: readonly string[],
): string {
  const everyone = listFormat(locale, { type: "conjunction" }).format([
    host,
    ...names,
  ]);
  return t("scheduling.teamLinks.defaultTitle", { names: everyone });
}

type LinkWrite = Readonly<{ id?: string; title: string; members: string[] }>;

function useTeamLinkWrites() {
  const client = useQueryClient();
  const refresh = () => client.invalidateQueries({ queryKey: LINKS_KEY });
  const save = useMutation({
    mutationFn: async ({ id, title, members }: LinkWrite) => {
      const body = {
        title,
        team_members: members.map((user_id) => ({ user_id })),
      };
      if (id === undefined) {
        unwrap(await api.POST("/scheduling/team-links", { body }));
        return;
      }
      unwrap(
        await api.PUT("/scheduling/team-links/{page_id}", {
          params: { path: { page_id: id } },
          body,
        }),
      );
    },
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/scheduling/team-links/{page_id}", {
          params: { path: { page_id: id } },
        }),
      );
    },
    onSuccess: refresh,
  });
  return { save, remove };
}

/**
 * The host's team links, each with its own address, beside the personal link.
 * Colleagues are named only here: the personal link offers the host's own
 * free time alone.
 */
export function TeamLinksCard({ hostName }: Readonly<{ hostName: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const links = useQuery({
    queryKey: LINKS_KEY,
    queryFn: async () => unwrap(await api.GET("/scheduling/team-links")),
  });
  const candidates = useQuery({
    queryKey: ["scheduling-team-candidates"],
    queryFn: async () => unwrap(await api.GET("/scheduling/team-candidates")),
  });
  const writes = useTeamLinkWrites();
  const named = (link: Link, members: readonly Member[]) => {
    const names = members.map((member) => nameOf(member, candidates.data));
    const derived = teamLinkTitle(
      t,
      locale,
      hostName,
      link.team_members.map((member) => nameOf(member, candidates.data)),
    );
    // A name the host never changed follows the team; a chosen one stays.
    const title =
      link.title === derived
        ? teamLinkTitle(t, locale, hostName, names)
        : link.title;
    writes.save.mutate({
      id: link.id,
      title,
      members: members.map((member) => member.user_id),
    });
  };
  return (
    <Panel
      title={t("scheduling.teamLinks.title")}
      footer={
        <span className="t-caption">{t("scheduling.teamLinks.foot")}</span>
      }
    >
      <PanelBody className="form-stack">
        {(links.data ?? []).map((link) => (
          <TeamLinkRows
            key={link.id}
            link={link}
            candidates={candidates.data ?? []}
            pending={writes.save.isPending || writes.remove.isPending}
            onMembers={(members) => named(link, members)}
            onDelete={() => writes.remove.mutate(link.id)}
          />
        ))}
        <SettingList>
          <SettingRow
            label={t("scheduling.teamLinks.create")}
            description={t("scheduling.teamLinks.createHelp")}
            control={(control) => (
              <Select
                aria-labelledby={control["aria-labelledby"]}
                aria-describedby={control["aria-describedby"]}
                options={(candidates.data ?? []).map((candidate) => ({
                  value: candidate.user_id,
                  label: candidate.name,
                }))}
                value=""
                placeholder={t(
                  candidates.isSuccess && candidates.data.length === 0
                    ? "scheduling.team.noneToAdd"
                    : "scheduling.team.choose",
                )}
                disabled={
                  !candidates.data?.length ||
                  writes.save.isPending ||
                  !links.isSuccess
                }
                onChange={(user) =>
                  writes.save.mutate({
                    title: teamLinkTitle(t, locale, hostName, [
                      nameOf({ user_id: user }, candidates.data),
                    ]),
                    members: [user],
                  })
                }
              />
            )}
          />
        </SettingList>
        <ErrorLine
          error={
            writes.save.error ??
            writes.remove.error ??
            links.error ??
            candidates.error
          }
        />
      </PanelBody>
    </Panel>
  );
}

function nameOf(member: Member, candidates: readonly Candidate[] | undefined) {
  return (
    member.name ??
    candidates?.find((candidate) => candidate.user_id === member.user_id)
      ?.name ??
    ""
  );
}

// A link switched off takes no bookings whatever its blocker says.
function linkStatus(link: Link): LinkStatus {
  if (!link.enabled) return OFF;
  return LINK_STATUS[link.blocker] ?? PAUSED;
}

function TeamLinkRows({
  link,
  candidates,
  pending,
  onMembers,
  onDelete,
}: Readonly<{
  link: Link;
  candidates: readonly Candidate[];
  pending: boolean;
  onMembers: (members: readonly Member[]) => void;
  onDelete: () => void;
}>) {
  const t = useT();
  const [deleting, setDeleting] = useState(false);
  const members = link.team_members;
  const named = new Set(members.map((member) => member.user_id));
  const offered = candidates
    .filter((candidate) => !named.has(candidate.user_id))
    .map((candidate) => ({ value: candidate.user_id, label: candidate.name }));
  const full = members.length >= MAX_MEMBERS;
  const status = linkStatus(link);
  return (
    <SettingList>
      <SettingRow
        label={link.title}
        description={<StatusBadge kind={status.kind} label={t(status.key)} />}
        control={
          <div className="cell-actions">
            <CopyTeamLink url={link.public_url ?? ""} />
            <Button pending={pending} onClick={() => setDeleting(true)}>
              {t("scheduling.teamLinks.delete")}
            </Button>
          </div>
        }
      />
      {members.map((member) => {
        const state = memberStatus(member);
        return (
          <SettingRow
            key={member.user_id}
            label={member.name ?? ""}
            description={<StatusBadge kind={state.kind} label={t(state.key)} />}
            control={
              <Button
                pending={pending}
                reason={
                  members.length === 1
                    ? t("scheduling.teamLinks.lastMember")
                    : undefined
                }
                onClick={() =>
                  onMembers(members.filter((m) => m.user_id !== member.user_id))
                }
              >
                {t("scheduling.team.remove")}
              </Button>
            }
          />
        );
      })}
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
              offered.length === 0
                ? "scheduling.team.noneToAdd"
                : "scheduling.team.choose",
            )}
            disabled={full || offered.length === 0 || pending}
            onChange={(user) => {
              const added = candidates.find((c) => c.user_id === user);
              onMembers([...members, { user_id: user, name: added?.name }]);
            }}
          />
        )}
      />
      <ConfirmModal
        open={deleting}
        onClose={() => setDeleting(false)}
        title={t("scheduling.teamLinks.deleteTitle", { title: link.title })}
        confirmLabel={t("scheduling.teamLinks.delete")}
        pending={pending}
        onConfirm={() => {
          setDeleting(false);
          onDelete();
        }}
      >
        <p>{t("scheduling.teamLinks.deleteHelp")}</p>
      </ConfirmModal>
    </SettingList>
  );
}

function CopyTeamLink({ url }: Readonly<{ url: string }>) {
  const t = useT();
  const copy = useClipboardCopy(url, {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  return (
    <>
      <Button disabled={!url} onClick={copy.copy}>
        <Copy aria-hidden />
        {copy.label}
      </Button>
      {copy.notice}
    </>
  );
}
