import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  Modal,
  OverflowMenu,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem, useMe } from "./common";
import "./users-admin.css";
import { useHoldsAdminRole } from "../app/capability";
import { isOption } from "../app/options";
import {
  InviteUserForm,
  ROLES,
  type Role,
  roleLabel,
  roleOptions,
} from "./users-invite-form";
import { PasswordLinkModal, usePasswordLink } from "./users-password-link";

type User = components["schemas"]["User"];
// The member roster (org settings). Every user-management WRITE is admin-only
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
  const isAdmin = useHoldsAdminRole();
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
  // The ROSTER is not admin surface: `GET /users` answers 200 to any
  // authenticated principal, and "who is on my team and what may they do" is not
  // an admin's private question. So every seat gets the member list, and the two
  // things the server refuses them — inviting somebody, and changing a role or a
  // status — are the admin's. `probeSettled` is what keeps the read-only line
  // from flashing at an admin while /me is still in flight: a probe in flight is
  // not a denial.
  return (
    <MembersCard
      members={members}
      probeSettled={me.isSuccess}
      canIssueLink={canIssueLink}
      canAdminister={isAdmin}
    />
  );
}

function MembersCard({
  members,
  probeSettled,
  canIssueLink,
  canAdminister,
}: Readonly<{
  members: ReturnType<typeof useMembers>;
  probeSettled: boolean;
  canIssueLink: boolean;
  canAdminister: boolean;
}>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const roster = members.data;
  return (
    <Panel
      title={t("users.membersTitle")}
      // What the roster holds, and the one verb that adds to it, on the title's
      // own line — which is what this band is for. The count states the roster
      // INCLUDING deactivated members: the read opts into them, and a roster of
      // twelve with three switched off is not a roster of nine. Nothing to count
      // is said by the empty state below instead, so a "0 members" badge never
      // doubles it.
      titleAction={
        <>
          {roster && roster.length > 0 && (
            <Badge>
              {plural("users.memberCount", roster.length, {
                count: formatNumber(roster.length, locale),
              })}
            </Badge>
          )}
          {canAdminister && <InviteAction canIssueLink={canIssueLink} />}
        </>
      }
    >
      <PanelBody>
        {/* The card's description, and — for a seat that may not administer it —
            its read-only posture, in the one paragraph a reader starts on. The
            posture is stated ONCE for the whole card rather than beside each of
            the nine rows' worth of controls it refuses: withholding the page's
            one explanation is the defect, withholding twelve controls
            individually is noise (design-system/README.md §Absent, disabled, or
            withheld). */}
        <p className="settings-panel-sub">
          {t("users.membersSub")}
          {probeSettled && !canAdminister && ` ${t("users.adminOnly")}`}
        </p>
        <QueryGate query={members} pendingLabel={t("users.membersTitle")}>
          {(list) =>
            list.length === 0 ? (
              <EmptyState>{t("users.empty")}</EmptyState>
            ) : (
              // One SettingRow per member, so nine members read as nine lines
              // rather than nine cards. Before this a row drew the name, then a
              // full-width role Select on its own line, then two ghost buttons
              // under that — 140px per member, and a roster of nine was a
              // 1300px wall.
              <SettingList>
                {list.map((u) => (
                  <MemberRow
                    key={u.id}
                    member={u}
                    canIssueLink={canIssueLink}
                    canAdminister={canAdminister}
                  />
                ))}
              </SettingList>
            )
          }
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// Inviting somebody is four decisions committed together — an address, a name,
// a role and the teams they land in — so the roster's header carries the verb
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
      <Button small onClick={() => setOpen(true)}>
        {t("users.inviteOpen")}
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

// The role picker, held to the row language's measure so nine of them line up
// at one x rather than each shrinking to its own answer's width.
//
// It draws only where a role is something this reader can CHANGE. The agent
// seat has no role at all and a reader without the grant has a fact rather than
// a control — both read as the row's `value`, decided by the caller, because a
// picker the server would refuse promises something it cannot do.
function RoleCell({
  member,
  pending,
  inFlight,
  onPick,
}: Readonly<{
  member: User;
  pending: boolean;
  inFlight?: Role;
  onPick: (role: Role) => void;
}>) {
  const t = useT();
  // `roles` arrives only for an admin caller — which this control always has —
  // and normally holds exactly one key. No key (an unassigned seat) and several
  // keys both leave the select on its placeholder, because neither has one
  // current role to show.
  const heldRoles = member.roles ?? [];
  const currentRole =
    heldRoles.length === 1 && isOption(heldRoles[0], ROLES) ? heldRoles[0] : "";
  // A member holding SEVERAL roles is the case worth naming: any choice here
  // replaces the whole set, so a neutral "Set role…" would let an admin strip
  // privileges they never saw. The placeholder says what is held instead.
  // plural-rule:allow the two arms name what is held and what to do, which is a
  // state the reader is in rather than two forms of one sentence
  const placeholder =
    heldRoles.length > 1
      ? t("users.rolesHeld", { roles: heldRoles.map(roleLabel(t)).join(", ") })
      : t("users.setRole");
  return (
    // The unset state is the select's PLACEHOLDER, not an option: picking it
    // back would set no role, so it belongs on the closed face and nowhere in
    // the list. It is only ever seen when there is no single role to show.
    //
    // Its own `aria-label` rather than the row's label through `control`'s ARIA:
    // the row names the MEMBER, and a combobox announcing itself as "Ada
    // Active" would leave a reader to guess what picking from it does.
    <Select
      className="settingrow-measure"
      aria-label={t("users.setRoleFor", { name: member.display_name })}
      value={inFlight ?? currentRole}
      placeholder={placeholder}
      disabled={pending}
      onChange={(value) => {
        if (isOption(value, ROLES)) {
          onPick(value);
        }
      }}
      options={roleOptions(t)}
    />
  );
}

// The role a row reports rather than offers: the agent seat's, whose authority
// is the passport granting it intersected with the person that passport names,
// and any member's for a reader who may not change one. `undefined` means the
// row draws a picker instead.
function roleAnswer(
  member: User,
  canAdminister: boolean,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (member.is_agent) {
    return t("users.agentSeatRole");
  }
  if (canAdminister) {
    return undefined;
  }
  const held = (member.roles ?? []).map(roleLabel(t)).join(", ");
  // A seat holding no role has nothing to report, and an empty value would
  // draw an empty span where the answer belongs.
  return held === "" ? undefined : held;
}

// The verbs a member's row offers, behind the one control a row spends on them.
//
// Two of them, one a credential and one destructive, used to sit as ghost
// buttons stacked under the row — which is the greater part of why a member cost
// 140px. Neither is what a reader opens this roster for, and that is exactly
// what an OverflowMenu is for. Nothing to offer draws nothing: a trigger over an
// empty panel is a promise the row cannot keep.
function MemberVerbs({
  member,
  pending,
  canMintLink,
  canDeactivate,
  canReactivate,
  onMintLink,
  onDeactivate,
  onReactivate,
}: Readonly<{
  member: User;
  pending: boolean;
  canMintLink: boolean;
  canDeactivate: boolean;
  canReactivate: boolean;
  onMintLink: () => void;
  onDeactivate: () => void;
  onReactivate: () => void;
}>) {
  const t = useT();
  if (!(canMintLink || canDeactivate || canReactivate)) {
    return null;
  }
  return (
    <OverflowMenu label={t("users.rowActions", { name: member.display_name })}>
      {canMintLink && (
        <Button small disabled={pending} onClick={onMintLink}>
          {t("users.link.action")}
        </Button>
      )}
      {canDeactivate && (
        <Button small disabled={pending} onClick={onDeactivate}>
          {t("users.deactivate")}
        </Button>
      )}
      {canReactivate && (
        <Button small disabled={pending} onClick={onReactivate}>
          {t("users.reactivate")}
        </Button>
      )}
    </OverflowMenu>
  );
}

// statusTone maps a member's status onto the badge vocabulary.
//
// Invited is deliberately NEUTRAL rather than a warning. An invitation in
// flight is the ordinary first state of every seat, and painting a whole
// column of new colleagues amber tells an admin something is wrong when
// nothing is. Warn is kept for the states somebody has to act on.
function statusTone(status: string): "success" | "warn" | "danger" | undefined {
  switch (status) {
    case "active":
      return "success";
    case "invited":
      return undefined;
    default:
      return "warn";
  }
}

function MemberRow({
  member,
  canIssueLink,
  canAdminister,
}: Readonly<{
  member: User;
  canIssueLink: boolean;
  canAdminister: boolean;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [confirmOff, setConfirmOff] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  // Where the deactivate confirm hands focus back. The Deactivate item it was
  // opened from is gone by then — Reactivate has taken its place — but the row
  // itself stays, because the roster is read with include_inactive.
  const row = useRef<HTMLDivElement | null>(null);
  const passwordLink = usePasswordLink();
  const openLink = () => {
    setLinkOpen(true);
    void passwordLink.mint(member.id);
  };
  // Returns the refetch so each onSuccess can hand it back to react-query,
  // which then keeps the mutation pending until the new roster lands. Without
  // that the mutation settles first and the row renders the pre-change roster
  // it still has cached — the member's OLD role, briefly, right after a
  // successful change.
  const refresh = () => {
    setError(null);
    return qc.invalidateQueries({ queryKey: ["users-admin"] });
  };
  const onError = (e: Error) => setError(problemMessageOf(e, t));
  const toast = useToast();

  const setRole = useMutation({
    mutationFn: async (role: Role) => {
      const { error: err } = await api.PATCH("/users/{id}/role", {
        params: { path: { id: member.id } },
        body: { role },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      await refresh();
      // No Undo: reversing it means knowing which role they held before, and
      // the roster this row redraws from has already been replaced by the time
      // a reader could press it. A wrong role restored quietly is worse than
      // no offer at all.
      toast.show(t("users.roleSaved", { name: member.email }));
    },
    onError,
  });

  const deactivate = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST("/users/{id}/deactivate", {
        params: { path: { id: member.id } },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      // The refreshed roster FIRST, then the dialog: closing it hands focus back
      // to the row, and a row still showing "Active" beside a Deactivate item
      // would announce the state this confirm just ended.
      await refresh();
      setConfirmOff(false);
      // A true inverse, and both halves are already on this row: `reactivate`
      // restores exactly what `deactivate` took away, with nothing to re-supply.
      toast.show(t("users.deactivated", { name: member.email }), {
        action: { label: t("common.undo"), onAct: () => reactivate.mutate() },
      });
    },
    onError,
  });

  const reactivate = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST("/users/{id}/reactivate", {
        params: { path: { id: member.id } },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t("users.reactivated", { name: member.email }));
    },
    onError,
  });

  const pending =
    setRole.isPending || deactivate.isPending || reactivate.isPending;

  // Issuance admits exactly whom redemption admits — an active member, and an
  // invited one who has not redeemed yet — so a deactivated row is excluded
  // because the link would be dead on arrival. The agent seat is excluded for a
  // different reason: it holds no password by construction, which is what makes
  // it a thing that signs in nowhere, and the server refuses to mint it one.
  //
  // Invited matters most here: an invitation whose link expired leaves a member
  // with no password, so the self-service reset refuses them and this is the
  // only route back into the account. Withholding it would strand them.
  const canMintLink =
    canAdminister &&
    canIssueLink &&
    !member.is_agent &&
    (member.status === "active" || member.status === "invited");
  // Offered on an invitation too, and that is not cosmetic: an unredeemed
  // invitation holds a licensed seat, so without this an invitation sent to the
  // wrong address consumes a seat with no way to release it.
  const canDeactivate =
    canAdminister &&
    (member.status === "active" || member.status === "invited");
  const canReactivate = canAdminister && member.status === "deactivated";

  return (
    // The row's own wrapper, so a refusal reads UNDER the member it belongs to
    // and inside their cell — the list's hairline still separates one member
    // from the next. tabIndex -1 makes it reachable by focus() without putting
    // a container into anybody's Tab order; the deactivate confirm below is the
    // only thing that focuses it.
    <div data-testid={`member-${member.id}`} ref={row} tabIndex={-1}>
      <SettingRow
        label={member.display_name}
        description={member.email}
        value={roleAnswer(member, canAdminister, t)}
        // Status, then role, then the verbs — and that ORDER is what keeps nine
        // role pickers at one x. The control column packs from the right, so an
        // item's position is decided by the width of everything after it: with
        // the badge last, "Deactivated" pushed that row's picker 34px left of
        // the other eight. Only the menu trigger, which is one glyph wide on
        // every row, sits after the picker now.
        control={
          <>
            <Badge tone={statusTone(member.status)}>
              {t(`users.status.${member.status}`)}
            </Badge>
            {/* The workspace's agent identity sits in this roster because it
                OWNS records — a client resolving an owner has to find it — so
                the row says what it is rather than passing for a colleague. */}
            {member.is_agent && <Badge tone="ai">{t("users.agentSeat")}</Badge>}
            {canAdminister && !member.is_agent && (
              <RoleCell
                member={member}
                pending={pending}
                // While a change is in flight the cell shows the role being
                // applied — and it stays in flight until the refreshed roster
                // lands (see refresh), so the row never renders the replaced
                // role. A FAILED change leaves it on the role still held, which
                // is what keeps a retry live: re-picking the same target still
                // fires onChange.
                inFlight={setRole.isPending ? setRole.variables : undefined}
                onPick={(role) => setRole.mutate(role)}
              />
            )}
            <MemberVerbs
              member={member}
              pending={pending}
              canMintLink={canMintLink}
              canDeactivate={canDeactivate}
              canReactivate={canReactivate}
              onMintLink={openLink}
              onDeactivate={() => setConfirmOff(true)}
              onReactivate={() => reactivate.mutate()}
            />
          </>
        }
      />
      {/* Same vocabulary as the invite dialog's refusal: a failed role change or
          deactivation is the surface saying something is wrong, and it takes
          the row's full width rather than wedging itself between the controls
          that caused it. */}
      {error && (
        <Callout tone="danger" live="alert" className="users-member-error">
          {error}
        </Callout>
      )}
      <ConfirmModal
        open={confirmOff}
        onClose={() => setConfirmOff(false)}
        title={t("users.deactivateConfirmTitle", { name: member.display_name })}
        confirmLabel={t("users.deactivate")}
        confirmVariant="danger"
        pending={deactivate.isPending}
        error={deactivate.error ? problemMessageOf(deactivate.error, t) : null}
        onConfirm={() => deactivate.mutate()}
        // The member's own row, which reads back their name, address and the
        // status this confirm just changed — the outcome, at the place the
        // operator was working. The item they pressed is not an option: a
        // deactivated row offers Reactivate instead, so the opener is gone.
        returnFocusTo={() => row.current}
      >
        {/* Deactivating the agent seat is a posture an operator is entitled to
            take, so it stays offered — and the generic body (signed out, sessions
            revoked) describes a person rather than an identity that signs in
            nowhere. The agent body's job is to say what does NOT stop: scheduled
            extension jobs keep running, because a tick acts as the job it is. */}
        <p className="t-caption">
          {t(
            member.is_agent
              ? "users.deactivateAgentConfirmBody"
              : "users.deactivateConfirmBody",
          )}
        </p>
      </ConfirmModal>
      {linkOpen && (
        <PasswordLinkModal
          memberName={member.display_name}
          link={passwordLink.state.link}
          pending={passwordLink.state.pending}
          error={passwordLink.state.error}
          onRetry={openLink}
          onClose={() => {
            // Drop the credential with the dialog, never merely hide it.
            passwordLink.clear();
            setLinkOpen(false);
          }}
        />
      )}
    </div>
  );
}
