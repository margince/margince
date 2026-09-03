// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import { navigate } from "../../app/router";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useMe } from "../common";
import { EMPTY_DRAFT } from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import { type InvitedMember, InviteUserForm } from "../users-invite-form";
import { PasswordLinkModal, usePasswordLink } from "../users-password-link";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import type { WizardPersistInput } from "./use-wizard-state";
import { ConversationWorkbench } from "./workbench";

// The team act: a creator who will not work in Margince names the first person
// who will. The form is the one Settings → People uses, so an invite sent from
// here is exactly an invite sent from there — role, teams and the set-password
// link included. On an installation with no outbound email the link is the
// only way the invited person ever gets in, so it is minted the moment the
// invite lands, the way the roster does it.
//
// Leaving is a finish, with or without an invite: step "complete" is written
// before the handoff plays, the same order the connect act keeps, because a
// navigation that outruns the write leaves the next reload back in the
// journey.

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
  const passwordLink = usePasswordLink();
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  const [entering, setEntering] = useState(false);
  // The server answers whether THIS caller can mint set-password links: admin,
  // on an installation with no email channel and a configured base URL.
  const canIssueLink = me.data?.admin_password_link ?? false;

  const finish = async () => {
    setFinishing(true);
    setFinishFailed(false);
    // The personal steps were recorded as skipped on the way in (the invite
    // checkpoint); this write only closes the journey.
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
    setEntering(true);
  };

  if (entering || state.phase === "tm.done") {
    return <BuildScene onDone={() => navigate({ screen: "home" })} />;
  }

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
        <div className="ob-scene-foot">
          <p className="ob-conv-notice" role="alert">
            {finishFailed ? t("ob.conv.team.persistFailed") : null}
          </p>
          <div className="ob-scene-foot-acts">
            {/* One way out, named for what it is: a skip while nobody has been
                invited, a finish once somebody has. */}
            <Button
              variant={invited.length > 0 ? "primary" : "ghost"}
              onClick={() => void finish()}
              disabled={finishing}
            >
              {t(
                invited.length > 0
                  ? "ob.conv.team.finish"
                  : "ob.conv.team.skip",
              )}
            </Button>
          </div>
        </div>
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
