// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHO CAN SEE THIS RECORD, in full: every colleague who can open the contact or
// company, grouped by why, with who can also change it. The "Shared with" list
// below it names only manual shares, so on an ordinary record it read "nothing
// here" while the whole company could open it.
//
// The server judges each colleague by the record's own read and edit gates;
// this file only groups and words the answer. It is refetched after every
// successful write (app/queryclient.ts) and when the earliest share lapses.
// Role, team and seat changes made elsewhere show on the next load. A reader
// outside member administration is shown every colleague judged without their
// teams, and a count of the colleagues a team adds.

import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { Avatar, Badge, Disclosure } from "../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelRow,
} from "../design-system/panel";
import { RoleBadge } from "../design-system/rbac";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { LoadMoreButton, QueryGate, throwProblem } from "./common";
import "./recordaccesspanel.css";

type RecordAccessAnswer = components["schemas"]["RecordAccess"];
type Member = components["schemas"]["RecordAccessMember"];
type Reason = components["schemas"]["RecordAccessReason"];
type Group = Member["group"];
export type AccessKind = "contact" | "company";

export function isAccessKind(kind: string): kind is AccessKind {
  return kind === "contact" || kind === "company";
}

export function recordAccessKey(kind: AccessKind, id: string) {
  return ["record-access", kind, id] as const;
}

// One page as wide as the contract allows; a larger company loads more.
const PAGE = 200;

async function fetchAccess(
  kind: AccessKind,
  id: string,
  cursor: string | null,
): Promise<RecordAccessAnswer> {
  const params = {
    path: { id },
    query: { limit: PAGE, ...(cursor ? { cursor } : {}) },
  };
  const { data, error } =
    kind === "contact"
      ? await api.GET("/contacts/{id}/access", { params })
      : await api.GET("/companies/{id}/access", { params });
  if (error) {
    throwProblem(error);
  }
  return data;
}

const GROUP_TITLE: Record<Exclude<Group, "everyone">, MessageKey> = {
  owner: "whoCanSee.group.owner",
  shared: "whoCanSee.group.shared",
  team_shared: "whoCanSee.group.teamShared",
};

type Words = Readonly<{
  t: ReturnType<typeof useT>;
  date: (iso: string) => string;
}>;

// One phrase per reason, the same on a colleague's row and on the reader's own
// line.
function reasonPhrase(reason: Reason, { t, date }: Words): string {
  const until = reason.expires_at
    ? ` ${t("whoCanSee.until", { date: date(reason.expires_at) })}`
    : "";
  switch (reason.code) {
    case "workspace_visible":
      return t("whoCanSee.reason.workspace");
    case "owner":
      return t("whoCanSee.reason.owner");
    case "user_share":
      return t("whoCanSee.reason.userShare") + until;
    case "team_share":
      return (
        (reason.team_name
          ? t("whoCanSee.reason.teamShareNamed", { team: reason.team_name })
          : t("whoCanSee.reason.teamShare")) + until
      );
    case "same_team_as_owner":
      return t("whoCanSee.reason.sameTeam");
    case "all_records":
      return t("whoCanSee.reason.allRecords");
    case "write_share":
      return t("whoCanSee.reason.writeShare") + until;
  }
}

function phrases(reasons: Reason[], words: Words): string {
  return [...new Set(reasons.map((r) => reasonPhrase(r, words)))].join(", ");
}

function MemberRow({
  member,
  words,
}: Readonly<{ member: Member; words: Words }>) {
  const { t } = words;
  return (
    <PanelRow className="who-can-see-row">
      <Avatar name={member.display_name} identity={member.user_id} />
      <div className="who-can-see-who">
        <span>{member.display_name}</span>
        <span className="t-caption">
          {phrases(
            member.can_change ? member.change_reasons : member.read_reasons,
            words,
          )}
        </span>
      </div>
      {member.roles?.map((role) => (
        <RoleBadge key={role} roleKey={role} />
      ))}
      {member.can_change && (
        <Badge tone="accent">{t("whoCanSee.canChange")}</Badge>
      )}
    </PanelRow>
  );
}

function YouLine({
  answer,
  kind,
  words,
}: Readonly<{ answer: RecordAccessAnswer; kind: AccessKind; words: Words }>) {
  const { t } = words;
  const you = answer.you;
  const verdict = you.can_change
    ? t(`whoCanSee.you.change.${kind}`)
    : t(`whoCanSee.you.open.${kind}`);
  const why = phrases(
    you.can_change ? you.change_reasons : you.read_reasons,
    words,
  );
  return (
    <p data-testid="who-can-see-you">
      {verdict} <span className="t-caption">{why}</span>
    </p>
  );
}

// The first page carries the counts and the reader's own line; every loaded
// page adds its rows.
function AccessBody({
  pages,
  kind,
  words,
  more,
}: Readonly<{
  pages: RecordAccessAnswer[];
  kind: AccessKind;
  words: Words;
  more: ReactNode;
}>) {
  const { t } = words;
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (n: number) => ({ count: formatNumber(n, locale) });
  const answer = pages[0];
  const rows = pages.flatMap((page) => page.data);
  const byGroup = (group: Group) => rows.filter((m) => m.group === group);
  const everyone = byGroup("everyone");
  return (
    <>
      <PanelBody>
        <YouLine answer={answer} kind={kind} words={words} />
        {answer.archived && (
          <p className="t-caption">{t("whoCanSee.archived")}</p>
        )}
      </PanelBody>
      {(Object.keys(GROUP_TITLE) as Exclude<Group, "everyone">[]).map(
        (group) => {
          const rows = byGroup(group);
          return (
            rows.length > 0 && (
              <div key={group} data-testid={`who-can-see-${group}`}>
                <PanelGroupHead title={t(GROUP_TITLE[group])} level="h3" />
                {rows.map((m) => (
                  <MemberRow key={m.user_id} member={m} words={words} />
                ))}
              </div>
            )
          );
        },
      )}
      {answer.group_counts.everyone > 0 && (
        <PanelBody>
          <Disclosure
            summary={plural(
              `whoCanSee.everyone.${kind}`,
              answer.group_counts.everyone,
              count(answer.group_counts.everyone),
            )}
          >
            <div data-testid="who-can-see-everyone">
              {everyone.map((m) => (
                <MemberRow key={m.user_id} member={m} words={words} />
              ))}
            </div>
          </Disclosure>
        </PanelBody>
      )}
      <PanelBody>
        {more}
        {answer.team_access_count > 0 && (
          <p className="t-caption" data-testid="who-can-see-team-access">
            {plural(
              "whoCanSee.teamAccess",
              answer.team_access_count,
              count(answer.team_access_count),
            )}
          </p>
        )}
        <p className="t-caption">{t(`whoCanSee.emails.${kind}`)}</p>
      </PanelBody>
    </>
  );
}

// When to read the answer again for a share that lapses: at the lapse, but
// never sooner than a minute from now, so a client clock running ahead of the
// server cannot turn it into a loop, and never later than an hour, which also
// keeps the delay inside what a timer can hold.
export const REFRESH_FLOOR_MS = 60_000;
export const REFRESH_CEILING_MS = 60 * 60_000;

function refreshDelay(refreshAt: string, now: number): number {
  const until = Date.parse(refreshAt) - now;
  return Math.min(REFRESH_CEILING_MS, Math.max(REFRESH_FLOOR_MS, until));
}

/**
 * The panel, for a contact or a company. The answer 404s for a reader who
 * cannot open the record, which the share screen has already refused.
 */
export function WhoCanSeePanel({
  kind,
  recordId,
}: Readonly<{ kind: AccessKind; recordId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const queryClient = useQueryClient();
  const query = useInfiniteQuery({
    queryKey: recordAccessKey(kind, recordId),
    initialPageParam: FIRST_PAGE,
    queryFn: ({ pageParam }) => fetchAccess(kind, recordId, pageParam),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  // Armed again after every read and counted from it, so a read that came back
  // before the lapse (the same refresh_at) still schedules the next one.
  const refreshAt = query.data?.pages[0]?.refresh_at;
  const readAt = query.dataUpdatedAt;
  useEffect(() => {
    if (!refreshAt) {
      return undefined;
    }
    const timer = setTimeout(
      () => {
        void queryClient.invalidateQueries({
          queryKey: recordAccessKey(kind, recordId),
        });
      },
      refreshDelay(refreshAt, readAt),
    );
    return () => clearTimeout(timer);
  }, [refreshAt, readAt, kind, recordId, queryClient]);

  const words: Words = { t, date: (iso) => formatDate(iso, locale, zone) };
  return (
    <Panel title={t("whoCanSee.title")}>
      <QueryGate query={query} pendingLabel={t("whoCanSee.title")}>
        {(data) => (
          <AccessBody
            pages={data.pages}
            kind={kind}
            words={words}
            more={<LoadMoreButton query={query} />}
          />
        )}
      </QueryGate>
    </Panel>
  );
}
