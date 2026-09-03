// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import { navigate } from "../../app/router";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { EMPTY_DRAFT } from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import type { WizardPersistInput } from "./use-wizard-state";
import { ConversationWorkbench } from "./workbench";

// The invite: the company is confirmed, and the two steps left — training a
// voice, connecting an inbox and a calendar — are about the PERSON answering,
// not the installation. So the question is whether that person will work in
// Margince at all. An administrator setting it up for a team finishes here,
// and nobody is walked through steps that could only ever be skipped.
//
// Declining is a finish, not a skip: it writes step "complete" itself, with
// both personal steps recorded as skipped, before the handoff plays — the
// same order the connect act keeps, because a navigation that outruns the
// write leaves the next reload back in the journey.

type InviteActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
}>;

export function InviteAct({ state, dispatch, persist }: InviteActProps) {
  const t = useT();
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  const [entering, setEntering] = useState(false);

  const decline = async () => {
    setFinishing(true);
    setFinishFailed(false);
    const persisted = await persist({
      step: "complete",
      values: EMPTY_DRAFT.values,
      voiceSkipped: true,
      connectSkipped: true,
    });
    setFinishing(false);
    if (!persisted) {
      setFinishFailed(true);
      return;
    }
    dispatch({ type: "INVITE_DECLINED" });
    setEntering(true);
  };

  if (entering || state.phase === "in.declined") {
    return <BuildScene onDone={() => navigate({ screen: "home" })} />;
  }

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      title={t("ob.conv.invite.title")}
      sub={t("ob.conv.invite.body")}
    >
      <div className="ob-scene ob-invite-scene">
        <ul className="ob-invite-offers">
          <li>{t("ob.conv.invite.voice")}</li>
          <li>{t("ob.conv.invite.connect")}</li>
        </ul>
        {/* The scene's foot, the same one the voice scenes end in. The decline
            is the quieter of the two: it is the honest answer for an
            administrator, not a shortcut past a step. */}
        <div className="ob-scene-foot">
          <p className="ob-conv-notice" role="alert">
            {finishFailed ? t("ob.conv.invite.persistFailed") : null}
          </p>
          <div className="ob-scene-foot-acts">
            <Button
              variant="ghost"
              onClick={() => void decline()}
              disabled={finishing}
            >
              {t("ob.conv.invite.no")}
            </Button>
            <Button
              variant="primary"
              onClick={() => dispatch({ type: "INVITE_ACCEPTED" })}
              disabled={finishing}
            >
              {t("ob.conv.invite.yes")}
            </Button>
          </div>
        </div>
      </div>
    </ConversationWorkbench>
  );
}
