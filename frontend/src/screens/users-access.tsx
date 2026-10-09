import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components, operations } from "../api/schema";
import { useCan, useCanWrite, useHoldsAdminRole } from "../app/capability";
import { Button, EmptyState, OverflowMenu } from "../design-system/atoms";
import { AvatarStack } from "../design-system/avatarstack";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem, useMe } from "./common";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";
import {
  membersOf,
  NewTeamAction,
  RenameTeamAction,
  TeamMembersModal,
  type TeamUser,
} from "./team-dialogs";
import "./users-access.css";

// What a seat will see, said by the server. The invite form asks before the
// invite goes out; the answer is the evaluated policy — the same grants,
// masks and read classes the gates read — so the screen never interprets a
// role a second way.

type AccessPreview = components["schemas"]["AccessPreview"];
type Team = components["schemas"]["Team"];
// The preview's role comes off the OPERATION rather than a request schema: the
// endpoint is a GET, so its parameters are the query and there is no body shape
// to name.
type Role = operations["previewAccess"]["parameters"]["query"]["role"];

// The objects worth a line in the preview: the record kinds a rep works.
const PREVIEW_OBJECTS = [
  "contact",
  "company",
  "lead",
  "deal",
  "project",
] as const;

function useAccessPreview(role: Role, teamIds: string[]) {
  return useQuery({
    // `stable`, not the reader's collation: this is a cache key, so the same
    // set of teams has to spell the same key wherever it is read.
    queryKey: ["access-preview", role, [...teamIds].sort(stable).join(",")],
    queryFn: async (): Promise<AccessPreview> => {
      const { data, error } = await api.GET("/users/access-preview", {
        params: { query: { role, team_ids: teamIds } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

export function AccessPreviewPanel({
  role,
  teamIds,
}: Readonly<{ role: Role; teamIds: string[] }>) {
  const t = useT();
  const preview = useAccessPreview(role, teamIds);
  return (
    <div className="users-access-preview" aria-live="polite">
      <p>{t("users.access.title")}</p>
      <QueryGate query={preview} pendingLabel={t("users.access.title")}>
        {(access) => <AccessSummary access={access} />}
      </QueryGate>
    </div>
  );
}

function AccessSummary({ access }: Readonly<{ access: AccessPreview }>) {
  const t = useT();
  const verbs = (object: string): string => {
    const grant = access.objects?.[object];
    if (!grant?.read) {
      return t("users.access.none");
    }
    const parts = [t("users.access.read")];
    if (grant.create || grant.update) parts.push(t("users.access.write"));
    if (grant.delete) parts.push(t("users.access.delete"));
    return parts.join(" · ");
  };
  const teams = (access.teams ?? []).map((team) => team.name).join(", ");
  return (
    <ul className="users-access-list">
      <li>{t("users.access.identity")}</li>
      <li>
        {access.row_scope === "all"
          ? t("users.access.writesAll")
          : access.row_scope === "team"
            ? teams
              ? t("users.access.writesTeam", { teams })
              : t("users.access.writesTeamNone")
            : t("users.access.writesOwn")}
      </li>
      {PREVIEW_OBJECTS.map((object) => (
        <li key={object}>
          {t(`users.access.object.${object}` satisfies MessageKey)}:{" "}
          {verbs(object)}
        </li>
      ))}
      {(access.field_masks ?? []).map((mask) => (
        <li key={`${mask.object}.${mask.field}`}>
          {t("users.access.mask", {
            field: `${mask.object}.${mask.field}`,
            when:
              mask.condition === "always"
                ? t("users.access.maskAlways")
                : t("users.access.maskOutside"),
          })}
        </li>
      ))}
    </ul>
  );
}

// The name a team confirmation says, off the roster this card already holds.
// A team whose row has gone falls back to its id rather than an empty quote.
type RosterEntry = NonNullable<ReturnType<typeof useRoster>["data"]>[number];

function teamName(
  entries: readonly RosterEntry[] | undefined,
  id: string,
): string {
  const found = entries?.find((entry) => entry.id === id);
  return found && "name" in found ? found.name : id;
}

// The teams card: the workspace's teams, who is on each, and the verbs that
// change them. Membership decides who may edit whose records.
export function TeamsCard() {
  const t = useT();
  const toast = useToast();
  const qc = useQueryClient();
  const me = useMe();
  // teams.go: membership and archiving are the admin's alone. `useCanWrite`,
  // because the seat ceiling sits above RBAC and refuses a read seat these.
  const canCreateTeam = useCanWrite("team_admin", "create");
  const canRename = useCanWrite("team_admin", "update");
  const isAdmin = useHoldsAdminRole();
  const canEditTeam = canRename && isAdmin;
  // `team_ids` rides the roster only for `user_admin:read`; without it every
  // user reads as on no team, a false statement rather than a withholding.
  const canSeeMembership = useCan("user_admin", "read");
  const teams = useRoster("team", true);
  const teamsPartial = useRosterPartial("team", true);
  const users = useRoster("user", canSeeMembership);
  const usersPartial = useRosterPartial("user", canSeeMembership);
  const [openTeamId, setOpenTeamId] = useState<string | null>(null);
  // It takes the name as well as the id. The archive invalidated ["teams"], so
  // by the time Undo is pressed the row may be gone and a lookup would miss.
  const restore = useMutation({
    mutationFn: async ({ id }: { id: string; name: string }) => {
      const { error } = await api.PATCH("/teams/{id}", {
        params: { path: { id } },
        body: { archived: false },
      });
      if (error) throwProblem(error);
    },
    // Sticky: the Undo it came from is consumed by the press, so a quiet
    // refusal would read as the team having come back.
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
    },
    onSuccess: (_restored, { name }) => {
      qc.invalidateQueries({ queryKey: ["teams"] });
      toast.show(t("users.teamRestored", { name }));
    },
  });
  const archive = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.PATCH("/teams/{id}", {
        params: { path: { id } },
        body: { archived: true },
      });
      if (error) throwProblem(error);
    },
    onSuccess: (_archived, id) => {
      // Read before the refetch lands, which takes the named row away.
      const name = teamName(teams.data, id);
      qc.invalidateQueries({ queryKey: ["teams"] });
      toast.show(t("users.teamArchived", { name }), {
        action: {
          kind: "undo",
          label: t("common.undo"),
          onAct: () => restore.mutate({ id, name }),
        },
      });
    },
  });
  const colleagues = (users.data ?? []).flatMap((entry) =>
    "email" in entry ? [entry] : [],
  );
  const verbs = { canRename, canArchive: canEditTeam };
  return (
    <Panel
      title={t("users.teamsTitle")}
      titleAction={canCreateTeam ? <NewTeamAction /> : undefined}
    >
      <PanelBody>
        <PanelIntro>
          {t("users.teamsSub")}
          {me.isSuccess &&
            !(canCreateTeam || canRename) &&
            ` ${t("users.teamsAdminOnly")}`}
          {me.isSuccess &&
            !canSeeMembership &&
            ` ${t("users.teamMembersAdminOnly")}`}
        </PanelIntro>
        {archive.isError && (
          <Callout tone="danger" kind="outcome" title={t("users.notArchived")}>
            {problemMessageOf(archive.error, t)}
          </Callout>
        )}
      </PanelBody>
      <QueryGate query={teams} pendingLabel={t("users.teamsTitle")}>
        {(list) => {
          const rows = list.flatMap((entry) =>
            "name" in entry ? [entry] : [],
          );
          if (rows.length === 0 && !teamsPartial) {
            return <EmptyState>{t("users.noTeamsYet")}</EmptyState>;
          }
          const open = rows.find((team) => team.id === openTeamId);
          return (
            <>
              <TeamTable
                rows={rows}
                colleagues={
                  canSeeMembership && users.isSuccess ? colleagues : null
                }
                verbs={verbs}
                archiving={archive.isPending}
                onArchive={(id) => archive.mutate(id)}
                onOpen={setOpenTeamId}
              />
              {open && canSeeMembership && (
                <TeamMembersModal
                  team={open}
                  users={colleagues}
                  usersPartial={usersPartial}
                  canEdit={canEditTeam}
                  open
                  onClose={() => setOpenTeamId(null)}
                />
              )}
            </>
          );
        }}
      </QueryGate>
      {teamsPartial && (
        <PanelBody>
          <RosterPartialNote partial={teamsPartial} />
        </PanelBody>
      )}
    </Panel>
  );
}

type TeamVerbs = Readonly<{ canRename: boolean; canArchive: boolean }>;

// One row per team. The name opens its members: a row-wide click would also
// catch a press on the row's menu, and a `<tr>` takes no keyboard focus.
function TeamTable({
  rows,
  colleagues,
  verbs,
  archiving,
  onArchive,
  onOpen,
}: Readonly<{
  rows: readonly Team[];
  /** The users the roster carries membership for; null while withheld. */
  colleagues: readonly TeamUser[] | null;
  verbs: TeamVerbs;
  archiving: boolean;
  onArchive: (id: string) => void;
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const names = new Map(rows.map((team) => [team.id, team.name]));
  const columns: DataTableColumn<Team>[] = [
    {
      key: "name",
      header: t("users.teamColumn"),
      render: (team) => {
        const parent = team.parent_team_id
          ? names.get(team.parent_team_id)
          : undefined;
        return (
          <CellStack>
            {colleagues ? (
              <button
                type="button"
                className="cell-link"
                aria-haspopup="dialog"
                onClick={() => onOpen(team.id)}
              >
                {team.name}
              </button>
            ) : (
              <span>{team.name}</span>
            )}
            {parent ? (
              <span className="t-caption">
                {t("users.teamParent", { name: parent })}
              </span>
            ) : null}
          </CellStack>
        );
      },
    },
    {
      key: "members",
      header: t("users.teamMembersColumn"),
      render: (team) => {
        const count = team.member_count ?? 0;
        const faces = colleagues ? membersOf(colleagues, team.id, locale) : [];
        return (
          <span className="users-team-faces">
            {faces.length > 0 ? (
              <AvatarStack
                contacts={faces.map((user) => ({
                  name: user.display_name,
                  identity: user.id,
                }))}
              />
            ) : null}
            <span className="users-team-count">
              {plural("users.teamMemberCount", count, {
                count: formatNumber(count, locale),
              })}
            </span>
          </span>
        );
      },
    },
  ];
  if (verbs.canRename || verbs.canArchive) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (team) => (
        <TeamMenu
          team={team}
          verbs={verbs}
          archiving={archiving}
          onArchive={() => onArchive(team.id)}
        />
      ),
    });
  }
  return (
    <DataTable
      fold
      bleed
      label={t("users.teamsTitle")}
      columns={columns}
      rows={[...rows]}
      rowKey={(team) => team.id}
    />
  );
}

function TeamMenu({
  team,
  verbs,
  archiving,
  onArchive,
}: Readonly<{
  team: Team;
  verbs: TeamVerbs;
  archiving: boolean;
  onArchive: () => void;
}>) {
  const t = useT();
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("users.teamRowActions", { name: team.name })}>
        {verbs.canRename && <RenameTeamAction team={team} />}
        {verbs.canArchive && (
          <Button variant="danger" disabled={archiving} onClick={onArchive}>
            {t("users.teamArchive")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}
