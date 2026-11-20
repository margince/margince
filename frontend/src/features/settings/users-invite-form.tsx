// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { UserPlus } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useHoldsAdminRole } from "../../lib/capability";
import { emailFault } from "../../lib/emailshape";
import { nameFromEmail } from "../../lib/namefromemail";
import {
  type FieldLabels,
  fieldCheckSentence,
  problemCodeOf,
  problemMessageOf,
  type RefusalSentences,
  refusalSentences,
  unwrap,
  writeMayHaveLanded,
} from "../../lib/problem";
import { roleOptions, useAssignableRoles } from "../../lib/roles.queries";
import { AccessPreviewPanel } from "./users-access";
import {
  entityOptions,
  useEntityChoice,
  useMayAssignEntity,
} from "./users-entitycell";
import { useMemberRefresh } from "./users-members";
import { MemberTeamsField } from "./users-teams";
import "./users-admin.css";

// The invite form: an address, a name, a role, the teams the member lands in
// and the legal entity they work for, committed together. Two surfaces ask it:
// the roster's dialog in Settings → Contacts and the setup journey's team step.
// It is one form so the two cannot come to invite differently. What happens after the write
// (closing a dialog, minting a set-password link, moving the journey on) is
// the caller's, through `onInvited`.

export type Role = components["schemas"]["ChangeUserRoleRequest"]["role"];

export type InvitedMember = Readonly<{ id: string; name: string }>;

// Refusals of an address another member holds, each with its own copy.
const EMAIL_CODES = new Set(["email_taken", "removed_member_email"]);

/**
 * A refused member write split between its fields and the foot. A taken
 * address answers 409 naming no field, and belongs on the email.
 */
export function memberRefusal(
  error: unknown,
  t: Translator,
  labels: FieldLabels,
): RefusalSentences {
  if (EMAIL_CODES.has(problemCodeOf(error) ?? "")) {
    return {
      byField: new Map([["email", problemMessageOf(error, t)]]),
      rest: null,
    };
  }
  return refusalSentences(error, t, labels);
}

/** What the member's name and address need before anything is sent. */
export function memberChecks(
  member: Readonly<{ email: string; display_name: string }>,
  labels: FieldLabels,
  locale: Locale,
): ReadonlyMap<string, string> {
  const found = new Map<string, string>();
  const email = member.email.trim();
  const malformed = emailFault(email, locale, labels.email);
  if (email === "") {
    found.set("email", fieldCheckSentence("required", locale, labels.email));
  } else if (malformed) {
    found.set("email", malformed);
  }
  if (member.display_name.trim() === "" && "display_name" in labels) {
    found.set(
      "display_name",
      fieldCheckSentence("required", locale, labels.display_name),
    );
  }
  return found;
}

export function InviteUserForm({
  titleId,
  askName = true,
  unconfirmedKey = "users.inviteUnconfirmed",
  onInvited,
}: Readonly<{
  /**
   * The heading's id, for a dialog to name itself by. Absent, the form draws
   * no heading of its own: a surface whose title already says "invite" does
   * not want it said twice.
   */
  titleId?: string;
  /**
   * Whether the form asks for the member's name. The setup journey does not:
   * one address is enough to get a first contact in, and the name is derived
   * from it (`nameFromEmail`) until somebody changes it.
   */
  askName?: boolean;
  /**
   * What the form says when an invite may have landed unseen. The roster's
   * dialog sits over the member list; the setup journey has none to point at.
   */
  unconfirmedKey?: MessageKey;
  onInvited: (member: InvitedMember) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const labels: FieldLabels = {
    email: t("users.emailLabel"),
    ...(askName
      ? {
          display_name: t("users.nameLabel"),
          greeting_name: t("users.greetingLabel"),
        }
      : null),
    role: t("users.roleLabel"),
    legal_entity_id: t("users.entityLabel"),
  };
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [greeting, setGreeting] = useState("");
  const assignable = useAssignableRoles(true);
  const offered = assignable.data ?? [];
  // The ordinary seat most colleagues hold, as the starting choice. A member
  // administrator whose own access does not cover it is not offered it, and
  // picks a role they are offered before the form sends anything.
  const [role, setRole] = useState<Role>("rep");
  const offersRole = offered.some((one) => one.key === role);
  const [teamIds, setTeamIds] = useState<string[]>([]);
  // Only an admin puts a member on a team (identity/teams.go), so only an
  // admin is asked which teams the new member joins.
  const placesOnTeams = useHoldsAdminRole();
  // Placing the member is assigning them, which asks the assignment's grants.
  const { entities, fallback } = useEntityChoice(useMayAssignEntity());
  const [entityId, setEntityId] = useState<string | null>(null);
  const placedIn = entityId ?? fallback?.id ?? null;
  const refreshMembers = useMemberRefresh();

  // Choices ride as variables: a closure can hold the previous render's.
  const displayName = askName ? name.trim() : nameFromEmail(email);
  // Asked only beside the name: the setup journey derives the name from the
  // address, and the member's first sign-in can fill this instead.
  const greetingName = askName ? greeting.trim() : "";
  const invite = useMutation({
    mutationFn: async (
      choice: Readonly<{
        email: string;
        name: string;
        role: Role;
        teams: string[];
        greeting: string;
        entity: string | null;
      }>,
    ): Promise<string> => {
      const data = unwrap(
        await api.POST("/users", {
          body: {
            email: choice.email,
            display_name: choice.name,
            ...(choice.greeting === ""
              ? {}
              : { greeting_name: choice.greeting }),
            role: choice.role,
            team_ids: choice.teams,
            ...(choice.entity === null
              ? {}
              : { legal_entity_id: choice.entity }),
          },
        }),
      );
      return data.id;
    },
    onSuccess: (newUserId, choice) => {
      setEmail("");
      setName("");
      setGreeting("");
      setRole("rep");
      setTeamIds([]);
      setEntityId(null);
      onInvited({ id: newUserId, name: choice.name });
    },
    onError: (error) => {
      faults.show(memberRefusal(error, t, labels).byField);
      if (writeMayHaveLanded(error)) void refreshMembers();
    },
  });
  const refused = memberRefusal(invite.error, t, labels);
  // A failure that may still have created the member sends the admin to the
  // list, refreshed, instead of to a retry. A refusal never may have landed.
  const foot = writeMayHaveLanded(invite.error)
    ? t(unconfirmedKey)
    : refused.rest;
  // An edit answers the refusal, so the foot's sentence goes with it.
  const edited = (field: string) => {
    faults.edited(field);
    invite.reset();
  };

  return (
    <>
      {titleId !== undefined && (
        <Heading size="large" className="t-h2 modal-title" id={titleId}>
          {t("users.inviteTitle")}
        </Heading>
      )}
      {/* A real <form>, so Enter submits it; the title above and the action
          row below sit outside it so a dialog can pin both while it scrolls. */}
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          if (invite.isPending) return;
          const member = { email: email.trim(), display_name: displayName };
          const checks = new Map(memberChecks(member, labels, locale));
          if (!offersRole) {
            checks.set(
              "role",
              fieldCheckSentence("required", locale, labels.role),
            );
          }
          if (faults.show(checks)) {
            invite.reset();
            return;
          }
          invite.mutate({
            email: member.email,
            name: member.display_name,
            role,
            teams: placesOnTeams ? teamIds : [],
            greeting: greetingName,
            // The default entity is what an invite naming none gets.
            entity: placedIn === fallback?.id ? null : placedIn,
          });
        }}
      >
        {titleId !== undefined && <p>{t("users.inviteSub")}</p>}
        <Field
          label={t("users.emailLabel")}
          required
          error={faults.of("email")}
        >
          {(control) => (
            <TextInput
              {...control}
              placeholder={t("users.emailPlaceholder")}
              type="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value);
                edited("email");
              }}
            />
          )}
        </Field>
        {askName && (
          <Field
            label={t("users.nameLabel")}
            required
            error={faults.of("display_name")}
          >
            {(control) => (
              <TextInput
                {...control}
                placeholder={t("users.namePlaceholder")}
                maxLength={255}
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  edited("display_name");
                }}
              />
            )}
          </Field>
        )}
        {askName && (
          <Field
            label={t("users.greetingLabel")}
            hint={t("users.greetingHint")}
            error={faults.of("greeting_name")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={greeting}
                onChange={(e) => {
                  setGreeting(e.target.value);
                  edited("greeting_name");
                }}
              />
            )}
          </Field>
        )}
        <Field label={t("users.roleLabel")} required error={faults.of("role")}>
          {(control) => (
            <Select
              {...control}
              value={offersRole ? role : ""}
              placeholder={t("users.setRole")}
              disabled={assignable.isPending}
              onChange={(next) => {
                setRole(next);
                edited("role");
              }}
              options={roleOptions(t, offered)}
            />
          )}
        </Field>
        {/* The teams the member joins on arrival. A team-scoped role with no
          team edits only its own records, and the preview below says so before
          the invite goes out. */}
        {placesOnTeams && (
          <MemberTeamsField
            value={teamIds}
            onChange={(next) => {
              setTeamIds(next);
              invite.reset();
            }}
            canManage
          />
        )}
        {fallback !== undefined && placedIn !== null && (
          <Field
            label={t("users.entityLabel")}
            error={faults.of("legal_entity_id")}
          >
            {(control) => (
              <Select
                {...control}
                value={placedIn}
                onChange={(next) => {
                  setEntityId(next);
                  edited("legal_entity_id");
                }}
                options={entityOptions(t, entities)}
              />
            )}
          </Field>
        )}
        {offersRole && <AccessPreviewPanel role={role} teamIds={teamIds} />}
        {/* ABOVE the submit row, where the sibling dialogs in this family put a
          refusal: under the button it reads as a footnote to the form rather
          than as the answer to the press. */}
        {foot !== null && <ErrorLine>{foot}</ErrorLine>}
      </form>
      {/* Outside a dialog (the onboarding team step) `.form-actions` aligns
          the row; inside one `.modal .actions` does. */}
      <div className="actions form-actions">
        <Button
          variant="primary"
          type="submit"
          form={formId}
          pending={invite.isPending}
        >
          <UserPlus aria-hidden /> {t("users.invite")}
        </Button>
      </div>
    </>
  );
}
