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
// Role, team and seat changes made elsewhere show on the next load.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
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
import { QueryGate, throwProblem } from "./common";
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

// One page as wide as the contract allows. A larger company says how many are
// not listed rather than paging a panel.
const PAGE = 200;

async function fetchAccess(
  kind: AccessKind,
  id: string,
): Promise<RecordAccessAnswer> {
  const params = { path: { id }, query: { limit: PAGE } };
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
      <Badge tone={member.can_change ? "accent" : undefined}>
        {t(member.can_change ? "whoCanSee.canChange" : "whoCanSee.canOpen")}
      </Badge>
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
      <b>{verdict}</b> {why}
    </p>
  );
}

function AccessBody({
  answer,
  kind,
  words,
}: Readonly<{ answer: RecordAccessAnswer; kind: AccessKind; words: Words }>) {
  const { t } = words;
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (n: number) => ({ count: formatNumber(n, locale) });
  const byGroup = (group: Group) =>
    answer.data.filter((m) => m.group === group);
  const everyone = byGroup("everyone");
  const unlisted =
    (answer.page.total ?? answer.data.length) - answer.data.length;
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
        {unlisted > 0 && (
          <p className="t-caption">
            {plural("whoCanSee.unlisted", unlisted, count(unlisted))}
          </p>
        )}
        <p className="t-caption">{t(`whoCanSee.emails.${kind}`)}</p>
      </PanelBody>
    </>
  );
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
  const query = useQuery({
    queryKey: recordAccessKey(kind, recordId),
    queryFn: () => fetchAccess(kind, recordId),
  });
  const refreshAt = query.data?.refresh_at;
  useEffect(() => {
    if (!refreshAt) {
      return undefined;
    }
    const wait = Math.max(0, Date.parse(refreshAt) - Date.now());
    const timer = setTimeout(() => {
      void queryClient.invalidateQueries({
        queryKey: recordAccessKey(kind, recordId),
      });
    }, wait);
    return () => clearTimeout(timer);
  }, [refreshAt, kind, recordId, queryClient]);

  const words: Words = { t, date: (iso) => formatDate(iso, locale, zone) };
  return (
    <Panel title={t("whoCanSee.title")}>
      <QueryGate query={query} pendingLabel={t("whoCanSee.title")}>
        {(answer) => <AccessBody answer={answer} kind={kind} words={words} />}
      </QueryGate>
    </Panel>
  );
}
