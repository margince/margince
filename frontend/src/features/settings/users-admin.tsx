import { useId, useState } from "react";
import {
  Avatar,
  Badge,
  Button,
  EmptyState,
  Modal,
} from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { CellStrip } from "../../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { formatDateAbbrev, formatDateTime } from "../../format/format";
import { formatRelativeTime } from "../../format/relativetime";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useMe } from "../../lib/session";
import "./users-admin.css";
import { PageHeadAction } from "../../components/pageheadaction";
import { useRoster } from "../../components/roster";
import { useCan, useCanWrite } from "../../lib/capability";
import {
  type AssignableRole,
  useAssignableRoles,
} from "../../lib/roles.queries";
import { usePageReadOnly } from "./settingsnav";
import { InviteUserForm } from "./users-invite-form";
import {
  memberAnchorId,
  type User,
  useMemberRefresh,
  useMembers,
} from "./users-members";
import { MemberVerbs } from "./users-memberverbs";
import { PasswordLinkModal, usePasswordLink } from "./users-password-link";
import { MemberRole, roleRefusal, withRoles } from "./users-rolecell";

export function UsersAdminCard() {
  const me = useMe();
  // `useCanWrite`: the seat ceiling refuses a read seat holding the grant. Every
  // verb needs the read too, or a role change replaces roles the reader cannot see.
  const administersRoster = useCan("user_admin", "read");
  const mayInvite = useCanWrite("user_admin", "create");
  const mayChangeRole = useCanWrite("user_admin", "update");
  return (
    <MembersCard
      me={me.data?.user.id}
      probeSettled={me.isSuccess}
      probing={me.isPending}
      // Only where no email channel exists; elsewhere the invite mail carries it.
      canIssueLink={me.data?.admin_password_link ?? false}
      management={administersRoster}
      canInvite={administersRoster && mayInvite}
      canChangeRole={administersRoster && mayChangeRole}
      readOnlyPage={usePageReadOnly("members")}
    />
  );
}

function MembersCard({
  me,
  probeSettled,
  probing,
  canIssueLink,
  management,
  canInvite,
  canChangeRole,
  readOnlyPage,
}: Readonly<{
  me: string | undefined;
  probeSettled: boolean;
  probing: boolean;
  canIssueLink: boolean;
  management: boolean;
  canInvite: boolean;
  canChangeRole: boolean;
  readOnlyPage: boolean;
}>) {
  const t = useT();
  const members = useMembers();
  const read = withRoles(
    members,
    useAssignableRoles(canChangeRole),
    canChangeRole,
  );
  // A reader who may change nothing gets the page's read-only notice. The card
  // speaks only for the partial posture that notice cannot see.
  const posture =
    probeSettled && !readOnlyPage && management && !canChangeRole
      ? t("users.role.withheld")
      : null;
  const loaded = read.data !== undefined && !probing;
  return (
    <>
      {loaded && canInvite && (
        <PageHeadAction>
          <InviteAction canIssueLink={canIssueLink} />
        </PageHeadAction>
      )}
      {/* Untitled: on a one-panel page the page heading names the roster. */}
      <Panel
        actions={members.hasNextPage && <LoadMoreButton query={members} />}
      >
        {posture && (
          <PanelBody>
            <PanelIntro>{posture}</PanelIntro>
          </PanelBody>
        )}
        {/* Until /me answers the role picker's read is not known to be needed, and
            drawing the roster early would drop back to pending once it is. */}
        {read.data === undefined || probing ? (
          <PanelBody>
            <QueryStates
              query={probing ? { ...read, isPending: true } : read}
              pendingLabel={t("users.membersTitle")}
            >
              {null}
            </QueryStates>
          </PanelBody>
        ) : read.data.list.length === 0 ? (
          <PanelBody>
            <EmptyState>{t("users.empty")}</EmptyState>
          </PanelBody>
        ) : (
          <MembersTable
            list={read.data.list}
            roles={read.data.roles}
            management={management}
            refusalOf={(member, list, roles) =>
              roleRefusal(member, {
                roster: list,
                roles,
                canChangeRole,
                meId: me,
                t,
              })
            }
          />
        )}
      </Panel>
    </>
  );
}

// Server order is join order (created_at, id), which the Added line reads
// back; re-sorting the first page here would misstate the pages after it.
function MembersTable({
  list,
  roles,
  management,
  refusalOf,
}: Readonly<{
  list: User[];
  roles: readonly AssignableRole[];
  management: boolean;
  refusalOf: (
    member: User,
    list: readonly User[],
    roles: readonly AssignableRole[],
  ) => ReturnType<typeof roleRefusal>;
}>) {
  const t = useT();
  const teamNames = useTeamNames(management);
  const member: DataTableColumn<User> = {
    key: "member",
    header: t("users.col.member"),
    render: (u) => <MemberIdentity member={u} />,
  };
  const activity: DataTableColumn<User> = {
    key: "activity",
    header: t("users.col.activity"),
    render: (u) => <MemberActivity member={u} management={management} />,
  };
  // Without `user_admin:read` the roster omits roles, teams, last activity and
  // verbs, so that reader sees only who each member is and when they joined.
  const columns: DataTableColumn<User>[] = management
    ? [
        member,
        {
          key: "role",
          header: t("users.roleLabel"),
          render: (u) => (
            <MemberRole
              member={u}
              roles={roles}
              refusal={refusalOf(u, list, roles)}
            />
          ),
        },
        {
          key: "teams",
          header: t("users.teamsLabel"),
          render: (u) => <MemberTeams ids={u.team_ids} names={teamNames} />,
        },
        activity,
        {
          key: "verbs",
          header: t("table.actions"),
          headerHidden: true,
          fold: "end",
          render: (u) => <MemberVerbs member={u} />,
        },
      ]
    : [member, activity];
  return (
    <DataTable
      fold
      bleed
      label={t("users.membersTitle")}
      columns={columns}
      rows={list}
      rowKey={(u) => u.id}
    />
  );
}

function useTeamNames(enabled: boolean): ReadonlyMap<string, string> {
  const teams = useRoster("team", enabled);
  const names = new Map<string, string>();
  for (const entry of teams.data ?? []) {
    if ("name" in entry) {
      names.set(entry.id, entry.name);
    }
  }
  return names;
}

function MemberIdentity({ member }: Readonly<{ member: User }>) {
  const t = useT();
  return (
    // Focusable by script only: the deactivate confirm hands focus back here.
    <span id={memberAnchorId(member.id)} className="users-member" tabIndex={-1}>
      <Avatar name={member.display_name} identity={member.id} />
      <CellStack>
        <span className="users-member-name">
          <span className="t-name">{member.display_name}</span>
          {/* The agent seat owns records, so it is listed; the badge keeps it
              from passing for a colleague. */}
          {member.is_agent && <Badge tone="ai">{t("users.agentSeat")}</Badge>}
          <MemberStatus status={member.status} />
        </span>
        <span className="t-caption">{member.email}</span>
      </CellStack>
    </span>
  );
}

function MemberTeams({
  ids,
  names,
}: Readonly<{
  ids: string[] | undefined;
  names: ReadonlyMap<string, string>;
}>) {
  const known = (ids ?? []).flatMap((id) => names.get(id) ?? []);
  if (known.length === 0) {
    return null;
  }
  return (
    <span className="users-teams" title={known.join(", ")}>
      <CellStrip>
        {known.map((name) => (
          <Badge key={name}>{name}</Badge>
        ))}
      </CellStrip>
    </span>
  );
}

// Active is every seat's ordinary state, so only the exceptions are marked.
// Invited and deactivated are settled facts; a suspension asks someone to act.
const STATUS_KIND = {
  invited: "neutral",
  suspended: "attention",
  deactivated: "neutral",
} as const satisfies Readonly<Record<string, StatusKind>>;

function MemberStatus({ status }: Readonly<{ status: string }>) {
  const t = useT();
  if (
    status !== "invited" &&
    status !== "suspended" &&
    status !== "deactivated"
  ) {
    return null;
  }
  return (
    <StatusBadge
      kind={STATUS_KIND[status]}
      label={t(`users.status.${status}`)}
    />
  );
}

function MemberActivity({
  member,
  management,
}: Readonly<{ member: User; management: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const added = member.created_at;
  return (
    <CellStack>
      {management && <LastActive at={member.last_active_at} />}
      {added && (
        <time className="t-caption users-nowrap" dateTime={added}>
          {t("users.addedOn", {
            date: formatDateAbbrev(added, locale, viewerZone()),
          })}
        </time>
      )}
    </CellStack>
  );
}

// Null means never signed in, or withheld from this reader: a delegated admin
// sees none for a member they do not outrank. The cell claims neither.
function LastActive({ at }: Readonly<{ at: string | null | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!at) {
    return <span className="sr-only">{t("users.lastActiveUnknown")}</span>;
  }
  return (
    <time
      className="users-nowrap"
      dateTime={at}
      title={formatDateTime(at, locale, viewerZone())}
    >
      {formatRelativeTime(at, locale)}
    </time>
  );
}

// Inviting is four decisions committed together, so the header carries the
// verb and the form (shared with the setup journey) lives in the dialog.
function InviteAction({ canIssueLink }: Readonly<{ canIssueLink: boolean }>) {
  const t = useT();
  const refresh = useMemberRefresh();
  const formTitleId = useId();
  const [open, setOpen] = useState(false);
  // With no email channel the invite alone leaves a member who cannot sign in,
  // so the link is minted at once. The member's menu keeps the verb.
  const [invited, setInvited] = useState<{ id: string; name: string } | null>(
    null,
  );
  const passwordLink = usePasswordLink();

  return (
    <>
      {/* Named for what it opens: the dialog's own submit reads "Invite". */}
      <Button onClick={() => setOpen(true)}>{t("users.inviteOpen")}</Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={formTitleId}
        intent="form"
      >
        <InviteUserForm
          titleId={formTitleId}
          onInvited={(member) => {
            // Closes on the write that landed: a refused invite keeps what was typed.
            setOpen(false);
            void refresh();
            if (canIssueLink) {
              setInvited(member);
              void passwordLink.mint(member.id);
            }
          }}
        />
      </Modal>
      {invited && (
        <PasswordLinkModal
          memberName={invited.name}
          link={passwordLink.state.link}
          pending={passwordLink.state.pending}
          error={passwordLink.state.error}
          onRetry={() => void passwordLink.mint(invited.id)}
          onClose={() => {
            // Drop the credential with the dialog, never merely hide it.
            passwordLink.clear();
            setInvited(null);
          }}
        />
      )}
    </>
  );
}
