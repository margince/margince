// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import { useT } from "../../i18n";
import { useMe } from "../common";
import { EMPTY_DRAFT } from "../onboarding";
import { type InvitedMember, InviteUserForm } from "../users-invite-form";
import { PasswordLinkModal, usePasswordLink } from "../users-password-link";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import type { WizardPersistInput } from "./use-wizard-state";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench } from "./workbench";

// The team act: a creator who will not work in Margince names the first person
// who will. The form is the one Settings → People uses, so an invite sent from
// here is exactly an invite sent from there — role, teams and the set-password
// link included. On an installation with no outbound email the link is the
// only way the invited person ever gets in, so it is minted the moment the
// invite lands, the way the roster does it.
//
// Leaving, with or without an invite, closes the journey: the personal steps
// were already recorded as skipped on the way in (the invite checkpoint), so
// the one write here is completion itself — a server fact before the handoff
// plays, and a refusal keeps the reader here with the way out still offered.

type TeamActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
}>;

export function TeamAct({ state, dispatch, persist }: TeamActProps) {
  const t = useT();
  const me = useMe();
  const [invited, setInvited] = useState<readonly InvitedMember[]>([]);
  const [linkFor, setLinkFor] = useState<InvitedMember | null>(null);
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  const passwordLink = usePasswordLink();
  // The server answers whether THIS caller can mint set-password links: admin,
  // on an installation with no email channel and a configured base URL.
  const canIssueLink = me.data?.admin_password_link ?? false;

  const finish = async () => {
    setFinishing(true);
    setFinishFailed(false);
    const persisted = await persist({
      step: "complete",
      values: EMPTY_DRAFT.values,
    });
    setFinishing(false);
    if (!persisted) {
      setFinishFailed(true);
      return;
    }
    dispatch({ type: "TEAM_DONE" });
  };

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      title={t("ob.conv.team.title")}
      sub={t("ob.conv.team.body")}
    >
      <div className="ob-scene ob-team-scene">
        <div className="ob-team-form">
          <InviteUserForm
            askName={false}
            onInvited={(member) => {
              setInvited((current) => [...current, member]);
              if (canIssueLink) {
                setLinkFor(member);
                void passwordLink.mint(member.id);
              }
            }}
          />
        </div>
        {invited.length > 0 && (
          <ul
            className="ob-team-invited"
            aria-label={t("ob.conv.team.invitedLabel")}
          >
            {invited.map((member) => (
              <li key={member.id}>
                {t("ob.conv.team.invitedLine", { name: member.name })}
              </li>
            ))}
          </ul>
        )}
        {/* One way out, named for what it is: a skip while nobody has been
            invited, a finish once somebody has. */}
        <WayOnward
          label={t(
            invited.length > 0 ? "ob.conv.team.finish" : "ob.conv.team.skip",
          )}
          variant={invited.length > 0 ? "primary" : "ghost"}
          pending={finishing}
          stillNeeded={(why) => why.join(" ")}
          note={
            finishFailed ? (
              <p className="ob-stage-note" role="alert">
                {t("ob.conv.team.persistFailed")}
              </p>
            ) : undefined
          }
          onGo={() => void finish()}
        />
      </div>
      {linkFor && (
        <PasswordLinkModal
          memberName={linkFor.name}
          link={passwordLink.state.link}
          pending={passwordLink.state.pending}
          error={passwordLink.state.error}
          onRetry={() => void passwordLink.mint(linkFor.id)}
          onClose={() => {
            // Drop the credential with the dialog, never merely hide it.
            passwordLink.clear();
            setLinkFor(null);
          }}
        />
      )}
    </ConversationWorkbench>
  );
}
