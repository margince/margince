import { useId, useState } from "react";
import {
  Avatar,
  Badge,
  Button,
  EmptyState,
  Modal,
} from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { CellStrip } from "../design-system/listtable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { formatDateAbbrev, formatDateTime } from "../format/format";
import { formatRelativeTime } from "../format/relativetime";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { QueryStates, useMe } from "./common";
import "./users-admin.css";
import { useCan, useCanWrite } from "../app/capability";
import { type AssignableRole, useAssignableRoles } from "./roles.queries";
import { useRoster } from "./roster";
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
  const maySetStatus = useCanWrite("user_admin", "delete");
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
      canSetStatus={administersRoster && maySetStatus}
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
  canSetStatus,
}: Readonly<{
  me: string | undefined;
  probeSettled: boolean;
  probing: boolean;
  canIssueLink: boolean;
  management: boolean;
  canInvite: boolean;
  canChangeRole: boolean;
  canSetStatus: boolean;
}>) {
  const t = useT();
  const postureId = useId();
  const read = withRoles(
    useMembers(),
    useAssignableRoles(canChangeRole),
    canChangeRole,
  );
  // The read-only posture is stated once for the card, and the refused role
  // pickers point at it rather than each repeating it.
  const administers = canInvite || canChangeRole || canSetStatus;
  let posture: string | null = null;
  if (probeSettled && !administers) {
    posture = t("users.adminOnly");
  } else if (probeSettled && management && !canChangeRole) {
    posture = t("users.role.withheld");
  }
  return (
    <Panel
      title={t("users.membersTitle")}
      titleAction={canInvite && <InviteAction canIssueLink={canIssueLink} />}
    >
      {posture && (
        <PanelBody>
          <PanelIntro>
            <span id={postureId}>{posture}</span>
          </PanelIntro>
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
              cardReasonId: postureId,
              meId: me,
              t,
            })
          }
        />
      )}
    </Panel>
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
  // The roster omits roles, teams, activity and verbs for a reader without
  // `user_admin:read`, so those columns would only ever be empty.
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
          <StatusBadge status={member.status} />
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
// Invited is work in flight, not a fault.
const STATUS_TONE = {
  invited: "info",
  suspended: "warning",
  deactivated: "default",
} as const;

function StatusBadge({ status }: Readonly<{ status: string }>) {
  const t = useT();
  if (
    status !== "invited" &&
    status !== "suspended" &&
    status !== "deactivated"
  ) {
    return null;
  }
  return (
    <Badge tone={STATUS_TONE[status]}>{t(`users.status.${status}`)}</Badge>
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
    return (
      <span>
        <span aria-hidden="true">—</span>
        <span className="sr-only">{t("users.lastActiveUnknown")}</span>
      </span>
    );
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
