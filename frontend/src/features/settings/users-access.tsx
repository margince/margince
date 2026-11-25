import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components, operations } from "../../api/schema";
import {
  RosterPartialNote,
  useRoster,
  useRosterPartial,
} from "../../components/entityref";
import { PageHeadAction } from "../../components/pageheadaction";
import { useEveryMember, useEveryTeam } from "../../components/roster";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { AvatarStack } from "../../design-system/avatarstack";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { Switch } from "../../design-system/switch";
import { stable } from "../../format/collate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite, useHoldsAdminRole } from "../../lib/capability";
import { ENTITY_KINDS } from "../../lib/entity";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { changeReach, readableKinds } from "./accessreach";
import { useKindList } from "./accesswords";
import { ArchiveTeamConfirm, useArchiveFlow } from "./team-archive";
import {
  isTeamSeat,
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

function useAccessPreview(role: Role, teamIds: string[]) {
  return useQuery({
    // `stable`, not the reader's collation: this is a cache key, so the same
    // set of teams has to spell the same key wherever it is read.
    queryKey: ["access-preview", role, [...teamIds].sort(stable).join(",")],
    queryFn: async (): Promise<AccessPreview> => {
      return unwrap(
        await api.GET("/users/access-preview", {
          params: { query: { role, team_ids: teamIds } },
        }),
      );
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
  const kindList = useKindList();
  const readable = readableKinds(access.objects);
  const teams = (access.teams ?? []).map((team) => team.name).join(", ");
  const reach = changeReach(access.objects, access.row_scope);
  return (
    <ul className="users-access-list">
      <li>
        {readable.length === 0
          ? t("users.access.readsNone")
          : t("users.access.reads", { kinds: kindList(readable) })}
      </li>
      <li>
        {reach === "none"
          ? t("users.access.writesNone")
          : reach === "all"
            ? t("users.access.writesAll")
            : reach === "team"
              ? teams
                ? t("users.access.writesTeam", { teams })
                : t("users.access.writesTeamNone")
              : t("users.access.writesOwn")}
      </li>
      {ENTITY_KINDS.map((object) => (
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

// The teams card: the workspace's teams, who is on each, and the verbs that
// change them. Membership decides who may edit whose records.
export function TeamsCard() {
  const t = useT();
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
  const list = useTeamRows(isAdmin);
  // The wide walk, because the Members page lists invited seats and a team
  // count that includes them must have the same seats behind it.
  const users = useEveryMember(canSeeMembership);
  const usersPartial = users.data?.partial === true;
  const [openTeamId, setOpenTeamId] = useState<string | null>(null);
  const { archive, restore, ...archiveFlow } = useArchiveFlow();
  const { listed, rows, partial: listedPartial } = list;
  const colleagues = (users.data?.entries ?? []).flatMap((entry) =>
    "email" in entry && isTeamSeat(entry) ? [entry] : [],
  );
  const verbs = { canRename, canArchive: canEditTeam };
  const empty = listed.isSuccess && rows.length === 0 && !listedPartial;
  const open = rows.find((team) => team.id === openTeamId);
  return (
    <>
      {list.loaded && canCreateTeam && (
        <PageHeadAction>
          <NewTeamAction />
        </PageHeadAction>
      )}
      {/* Untitled: on a one-panel page the page heading names the teams. */}
      <Panel
        actions={list.mayShowArchived && <ShowArchivedTeams list={list} />}
      >
        <PanelBody>
          <PanelIntro>
            {t("users.teamsSub")}
            {me.isSuccess &&
              !canSeeMembership &&
              ` ${t("users.teamMembersAdminOnly")}`}
          </PanelIntro>
          {archive.isError && !archiveFlow.confirming && (
            <Callout
              tone="danger"
              kind="outcome"
              title={t("users.notArchived")}
            >
              {problemMessageOf(archive.error, t)}
            </Callout>
          )}
          {!listed.isSuccess && (
            <QueryStates query={listed} pendingLabel={t("users.teamsTitle")}>
              {null}
            </QueryStates>
          )}
          {empty && <EmptyState>{t("users.noTeamsYet")}</EmptyState>}
        </PanelBody>
        {listed.isSuccess && !empty && (
          <TeamTable
            rows={rows}
            colleagues={canSeeMembership && users.isSuccess ? colleagues : null}
            verbs={verbs}
            writing={archive.isPending || restore.isPending}
            onArchive={archiveFlow.ask}
            onRestore={(team) => restore.mutate(team)}
            onOpen={setOpenTeamId}
          />
        )}
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
        {listedPartial && (
          <PanelBody>
            <RosterPartialNote partial={listedPartial} />
          </PanelBody>
        )}
      </Panel>
      <ArchiveTeamConfirm {...archiveFlow.confirm} />
    </>
  );
}

// The rows the Teams page lists: the live roster, or every team with the
// archived ones once an admin asks for them.
function useTeamRows(isAdmin: boolean) {
  // Listing archived teams needs only the read: a read seat sees them.
  const mayShowArchived = useCan("team_admin", "read") && isAdmin;
  const [showArchived, setShowArchived] = useState(false);
  const teams = useRoster("team", true);
  const teamsPartial = useRosterPartial("team", true);
  const every = useEveryTeam(mayShowArchived && showArchived);
  const wide = mayShowArchived && showArchived;
  const entries = wide ? every.data?.entries : teams.data;
  return {
    mayShowArchived,
    showArchived,
    setShowArchived,
    loaded: teams.data !== undefined,
    listed: wide ? every : teams,
    partial: wide ? every.data?.partial === true : teamsPartial,
    rows: (entries ?? []).flatMap((entry) => ("name" in entry ? [entry] : [])),
  };
}

// Kept while archived teams are asked for, so a failed read of them can still
// be turned back off.
function ShowArchivedTeams({
  list,
}: Readonly<{ list: ReturnType<typeof useTeamRows> }>) {
  const t = useT();
  if (!list.loaded && !list.showArchived) {
    return null;
  }
  return (
    <Switch
      label={t("users.showArchivedTeams")}
      hint={t("users.showArchivedTeamsSub")}
      checked={list.showArchived}
      onChange={list.setShowArchived}
    />
  );
}

type TeamVerbs = Readonly<{ canRename: boolean; canArchive: boolean }>;

// One row per team. The name opens its members: a row-wide click would also
// catch a press on the row's menu, and a `<tr>` takes no keyboard focus.
function TeamTable({
  rows,
  colleagues,
  verbs,
  writing,
  onArchive,
  onRestore,
  onOpen,
}: Readonly<{
  rows: readonly Team[];
  /** The users the roster carries membership for; null while withheld. */
  colleagues: readonly TeamUser[] | null;
  verbs: TeamVerbs;
  writing: boolean;
  onArchive: (team: Team) => void;
  onRestore: (team: Team) => void;
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
            {/* An archived team takes no members, so its editor stays shut. */}
            {colleagues && !team.archived_at ? (
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
            {team.archived_at ? (
              <Badge tone="warning">{t("users.teamArchivedBadge")}</Badge>
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
                  photo: user.avatar_url,
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
          writing={writing}
          onArchive={() => onArchive(team)}
          onRestore={() => onRestore(team)}
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
  writing,
  onArchive,
  onRestore,
}: Readonly<{
  team: Team;
  verbs: TeamVerbs;
  writing: boolean;
  onArchive: () => void;
  onRestore: () => void;
}>) {
  const t = useT();
  if (team.archived_at) {
    return verbs.canArchive ? (
      <span className="cell-actions">
        <Button disabled={writing} onClick={onRestore}>
          {t("users.teamRestore")}
        </Button>
      </span>
    ) : null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: team.name })}>
        {verbs.canRename && <RenameTeamAction team={team} />}
        {verbs.canArchive && (
          <Button variant="danger" disabled={writing} onClick={onArchive}>
            {t("users.teamArchive")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}
