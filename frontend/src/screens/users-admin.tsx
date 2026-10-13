// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  EmptyState,
  Modal,
  SearchField,
  SegmentedControl,
  TableScroll,
} from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { QueryGate, throwProblem, useMe } from "./common";
import "./users-admin.css";
import { useCan, useCanWrite } from "../app/capability";
import { type AssignableRole, useAssignableRoles } from "./roles.queries";
import { useRosterNames } from "./roster";
import { InviteUserForm } from "./users-invite-form";
import { MemberRow } from "./users-memberrow";
import { PasswordLinkModal, usePasswordLink } from "./users-password-link";
import { withRoles } from "./users-rolecell";

type User = components["schemas"]["User"];
// The member roster (company settings). Every user-management WRITE is admin-only
// server-side, but the read is not: `GET /users` answers 200 to any authenticated
// principal, so the list is fetched for everyone and only the controls that
// change a member are withheld. The read opts into inactive members
// (include_inactive, honored server-side only for an admin) so a deactivated
// member can be reactivated. First page only in V1; larger member lists paginate
// in a follow-up.
function useMembers() {
  return useQuery({
    queryKey: ["users-admin"],
    queryFn: async (): Promise<User[]> => {
      const { data, error } = await api.GET("/users", {
        params: { query: { include_inactive: true } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

export function UsersAdminCard() {
  const me = useMe();
  // The four verbs the server actually distinguishes, not one "is admin".
  // `user_admin` has been seeded and enforced since the roster's gates moved off
  // the literal role, and each of these is the grant its own endpoint takes:
  // invite is CREATE (invite.go), a role change is UPDATE (users.go
  // ChangeUserRole), and deactivate AND reactivate are both DELETE — one verb
  // for turning a seat off and back on, because they are the same authority
  // over the same thing.
  //
  // `useCanWrite`, not `useCan`: the seat ceiling is enforced ABOVE RBAC
  // (identity/admission.go), so a read seat holding the grant is still refused
  // every one of these. A control it was shown could only ever 403.
  // Every roster verb ANDs with the read, because the write needs what the read
  // carries and the server does not send it otherwise:
  //
  //   - `ChangeUserRole` REPLACES the whole role set with the one picked
  //     (users.go). Without the read the roster omits `roles`, `RoleCell` reads
  //     the missing field as [], and picking a role would silently drop every
  //     other role the target holds — destroying authority the reader could not
  //     see.
  //   - `include_inactive` is honoured only for a caller who passes the read
  //     (handlers_roster.go), so a delete holder without it never sees a
  //     deactivated member to reactivate, nor an invited one to switch off.
  //
  // Inviting does not strictly need it, but a roster this reader cannot read is
  // not a page to invite from either.
  // Each hook on its own line and called unconditionally: `&&` short-circuits,
  // and the number of hooks a render performs must not depend on an answer.
  const administersRoster = useCan("user_admin", "read");
  const mayInvite = useCanWrite("user_admin", "create");
  const mayChangeRole = useCanWrite("user_admin", "update");
  const maySetStatus = useCanWrite("user_admin", "delete");
  const canInvite = administersRoster && mayInvite;
  const canChangeRole = administersRoster && mayChangeRole;
  const canSetStatus = administersRoster && maySetStatus;
  // These decide the card. Each ROW offers only what the server lists in that
  // member's `allowed_actions`, which also knows the member.
  const members = useMembers();
  // The server answers whether THIS caller can mint set-password links: admin,
  // on an installation with no email channel and a configured base URL. Where
  // email works, the invite mail carries the link and this action would only
  // ever 409 — so it is not rendered at all.
  const canIssueLink = me.data?.admin_password_link ?? false;
  // ONE card, because there is one subject: the roster. Inviting is a verb ON
  // that roster, not a second subject, and it used to be a card of its own
  // whose title, whose only row's label and whose button all read "Invite a
  // member" — the same three words, three times, above a list nine members
  // long that a reader came here to read.
  //
  // `GET /users` answers 200 to any authenticated principal, so the share and
  // assignee pickers keep working for every seat. What that endpoint CONTAINS
  // is narrowed instead: role keys and the deactivated members ride the
  // privileged projection, which handlers_roster.go sends only to a caller
  // holding `user_admin:read`.
  //
  // This settings PAGE follows the same read, because a roster nobody may act
  // on is a directory rather than an administration surface. `probeSettled` is
  // what keeps the read-only line from flashing while /me is still in flight: a
  // probe in flight is not a denial.
  return (
    <MembersCard
      members={members}
      probeSettled={me.isSuccess}
      administersRoster={administersRoster}
      canIssueLink={canIssueLink}
      canInvite={canInvite}
      canChangeRole={canChangeRole}
      canSetStatus={canSetStatus}
    />
  );
}

type RosterFilter = "all" | "active" | "invited" | "deactivated";
const FILTERS: readonly RosterFilter[] = [
  "all",
  "active",
  "invited",
  "deactivated",
];

// Whether `member` belongs under `filter`. Suspended members count under All
// only: no filter names them, and hiding them from every view would lose them.
function inFilter(member: User, filter: RosterFilter): boolean {
  return filter === "all" || member.status === filter;
}

function matchesSearch(member: User, needle: string): boolean {
  return (
    needle === "" ||
    member.display_name.toLocaleLowerCase().includes(needle) ||
    member.email.toLocaleLowerCase().includes(needle)
  );
}

function MembersCard({
  members,
  probeSettled,
  administersRoster,
  canIssueLink,
  canInvite,
  canChangeRole,
  canSetStatus,
}: Readonly<{
  members: ReturnType<typeof useMembers>;
  probeSettled: boolean;
  administersRoster: boolean;
  canIssueLink: boolean;
  canInvite: boolean;
  canChangeRole: boolean;
  canSetStatus: boolean;
}>) {
  // Whether this reader administers the roster AT ALL, for the one read-only
  // line the card states once. A reader holding some verbs and not others is
  // not read-only, and telling them so beside controls they can use would be
  // false — the individual verbs decide what each row offers.
  const administers = canInvite || canChangeRole || canSetStatus;
  const t = useT();
  // The roles this reader may hand out: what the change-role dialog offers, and
  // the names custom roles are shown under.
  const assignable = useAssignableRoles(canChangeRole);
  const read = withRoles(members, assignable, canChangeRole);
  return (
    // Untitled: the page head already says "Members", and a panel band saying
    // it again sat between that head and the roster it names.
    <Panel
      // The read-only posture is stated ONCE for the whole card rather than
      // beside each row's withheld controls, which would repeat it per row.
      // The foot explains rows, so an empty roster draws none of it.
      footer={
        members.data !== undefined &&
        members.data.length > 0 && (
          <span className="t-caption">
            {t("users.roster.foot")}
            {probeSettled && !administers && ` ${t("users.roster.readOnly")}`}
          </span>
        )
      }
    >
      <QueryGate query={read} pendingLabel={t("users.roster.title")}>
        {({ list, roles }) => (
          <Roster
            list={list}
            roles={roles}
            administersRoster={administersRoster}
            canChangeRole={canChangeRole}
            invite={canInvite && <InviteAction canIssueLink={canIssueLink} />}
          />
        )}
      </QueryGate>
    </Panel>
  );
}

// The roster proper: a search and a status filter over one table. Both narrow
// the page already read; the roster is one page in V1, so nothing is fetched.
function Roster({
  list,
  roles,
  administersRoster,
  canChangeRole,
  invite,
}: Readonly<{
  list: readonly User[];
  roles: readonly AssignableRole[];
  administersRoster: boolean;
  canChangeRole: boolean;
  // The invite verb, or false where this reader may not invite.
  invite: ReactNode;
}>) {
  const t = useT();
  const me = useMe();
  const [filter, setFilter] = useState<RosterFilter>("all");
  const [search, setSearch] = useState("");
  // Team names for the Teams column. `team_ids` rides only the privileged
  // projection, so a reader without it asks for no names either.
  const teams = useRosterNames("team", administersRoster);
  const teamNames = new Map<string, string>();
  for (const entry of teams.data ?? []) {
    if ("name" in entry) {
      teamNames.set(entry.id, entry.name);
    }
  }
  const countOf = (option: RosterFilter) =>
    list.filter((member) => inFilter(member, option)).length;
  const counts: Record<RosterFilter, number> = {
    all: countOf("all"),
    active: countOf("active"),
    invited: countOf("invited"),
    deactivated: countOf("deactivated"),
  };
  const needle = search.trim().toLocaleLowerCase();
  const shown = list.filter(
    (member) => inFilter(member, filter) && matchesSearch(member, needle),
  );
  return (
    <>
      <PanelBody className="users-roster-toolbar">
        <SearchField
          aria-label={t("users.roster.searchLabel")}
          placeholder={t("users.roster.searchPlaceholder")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        {invite}
        <SegmentedControl
          label={t("users.roster.filterLabel")}
          options={FILTERS}
          value={filter}
          onChange={setFilter}
          counts={counts}
          labels={{
            all: t("users.roster.filter.all"),
            active: t("users.roster.filter.active"),
            invited: t("users.roster.filter.invited"),
            deactivated: t("users.roster.filter.deactivated"),
          }}
        />
      </PanelBody>
      {shown.length === 0 ? (
        <PanelBody>
          <EmptyState>
            {needle !== ""
              ? t("users.roster.noMatch")
              : filter === "all"
                ? t("users.roster.empty")
                : t(`users.roster.emptyFilter.${filter}`)}
          </EmptyState>
        </PanelBody>
      ) : (
        <TableScroll label={t("users.roster.title")}>
          <table className="table users-roster">
            <thead>
              <tr>
                <th scope="col">{t("users.roster.col.member")}</th>
                {administersRoster && (
                  <>
                    <th scope="col">{t("users.roster.col.role")}</th>
                    <th scope="col">{t("users.roster.col.teams")}</th>
                  </>
                )}
                <th scope="col">{t("users.roster.col.status")}</th>
                <th scope="col" className="users-roster-actions">
                  <span className="sr-only">
                    {t("users.roster.col.actions")}
                  </span>
                </th>
              </tr>
            </thead>
            {shown.map((member) => (
              <MemberRow
                key={member.id}
                member={member}
                roles={roles}
                isMe={member.id === me.data?.user.id}
                teamNames={administersRoster ? teamNames : null}
                canChangeRole={canChangeRole}
              />
            ))}
          </table>
        </TableScroll>
      )}
    </>
  );
}

// Inviting somebody is four decisions committed together (an address, a name,
// a role and the teams they land in), so the roster's toolbar carries the verb
// and the form (users-invite-form.tsx, shared with the setup journey) lives in
// the dialog behind it.
function InviteAction({ canIssueLink }: Readonly<{ canIssueLink: boolean }>) {
  const t = useT();
  const qc = useQueryClient();
  const formTitleId = useId();
  const [open, setOpen] = useState(false);
  // Where no email channel exists the invite alone leaves a member who cannot
  // sign in, so the dialog opens straight away and mints the link. The member
  // row keeps its own action, which is what makes a dismissed dialog
  // recoverable.
  const [invited, setInvited] = useState<{ id: string; name: string } | null>(
    null,
  );
  const passwordLink = usePasswordLink();

  return (
    <>
      {/* Named for what it opens, not for what it does: this button invites
          nobody, and the dialog's own submit reads "Invite". Two buttons with
          one name are ambiguous for a reader and for `getByRole` alike. */}
      <Button variant="primary" onClick={() => setOpen(true)}>
        {t("users.roster.inviteOpen")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={formTitleId}
      >
        <InviteUserForm
          titleId={formTitleId}
          onInvited={(member) => {
            // The dialog closes on the write that landed, never before it: a
            // refused invite has to leave the address and the name where the
            // admin typed them.
            setOpen(false);
            qc.invalidateQueries({ queryKey: ["users-admin"] });
            // An invite spends a seat, so the capacity readings move with it.
            qc.invalidateQueries({ queryKey: ["installation-seat-usage"] });
            qc.invalidateQueries({ queryKey: ["installation-license"] });
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
