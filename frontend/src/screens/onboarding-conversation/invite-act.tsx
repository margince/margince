// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { User, Users } from "lucide-react";
import type { Dispatch } from "react";
import { useState } from "react";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { useT } from "../../i18n";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench } from "./workbench";

// The invite: the company is confirmed, and the two steps left — training a
// voice, connecting an inbox and a calendar — are about the PERSON answering,
// not the installation. So the question is whether that person will work in
// Margince at all. Yes opens those steps; no opens the team act instead, where
// the first person who will is invited, and nobody is walked through steps
// that could only ever be skipped.
//
// A ChoiceList rather than two buttons: the answers need explaining, and an
// answer that needs a line of its own is a choice to weigh, not a verb to
// press. The move onward is one Continue, so neither answer reads as the
// shortcut past the other.

type Answer = "yes" | "no";

export function InviteAct({
  state,
  dispatch,
}: Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
}>) {
  const t = useT();
  const [answer, setAnswer] = useState<Answer | "">("");
  const choices: readonly Choice<Answer>[] = [
    {
      value: "yes",
      label: t("ob.conv.invite.yes"),
      description: t("ob.conv.invite.yesBody"),
      mark: <User aria-hidden />,
    },
    {
      value: "no",
      label: t("ob.conv.invite.no"),
      description: t("ob.conv.invite.noBody"),
      mark: <Users aria-hidden />,
    },
  ];

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      title={t("ob.conv.invite.title")}
      sub={t("ob.conv.invite.body")}
    >
      <div className="ob-scene ob-invite-scene">
        <ChoiceList
          legend={t("ob.conv.invite.title")}
          hideLegend
          layout="cards"
          value={answer}
          choices={choices}
          onChange={setAnswer}
        />
        <div className="ob-scene-foot">
          <p>{t("ob.conv.invite.foot")}</p>
        </div>
        <WayOnward
          label={t("ob.conv.invite.continue")}
          blockers={answer === "" ? [t("ob.conv.invite.pickOne")] : []}
          stillNeeded={(why) => why.join(" ")}
          onGo={() =>
            dispatch({
              type: answer === "yes" ? "INVITE_ACCEPTED" : "INVITE_DECLINED",
            })
          }
        />
      </div>
    </ConversationWorkbench>
  );
}
