// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Dispatch } from "react";
import { useState } from "react";
import { navigate } from "../../app/router";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import { AutonomyChoices, useAutonomy } from "../autonomy-settings";
import { EMPTY_DRAFT } from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import type { WizardPersistInput } from "./use-wizard-state";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench } from "./workbench";

// The preferences act: the last word before the app. One thing is asked —
// what the agent may change on its own — prefilled from what the installation
// already holds. It is the reader's own, every seat has it, and each switch
// records itself the moment it moves, as it does in Settings, so a reader who
// agrees presses Done and nothing is written twice. The installation's
// reporting basis is not asked here: it was settled in the basis act, right
// after the company, before the steps about the person.
//
// Finishing is a server fact before it is a UI fact: step "complete" lands
// first, then the handoff plays.

type PrefsActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
}>;

export function PrefsAct({ state, dispatch, persist }: PrefsActProps) {
  const t = useT();
  const autonomy = useAutonomy();
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  const [entering, setEntering] = useState(false);

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
    dispatch({ type: "PREFS_DONE" });
    setEntering(true);
  };

  if (entering || state.phase === "pf.done") {
    return <BuildScene onDone={() => navigate({ screen: "home" })} />;
  }

  const stops = railStops(state.memberPath);
  const eyebrow = t("ob.conv.scene.step", {
    n: ordinalNumber(stops.findIndex((stop) => stop.key === "prefs") + 1),
    m: ordinalNumber(stops.length),
    label: t("ob.rail.prefs"),
  });
  const autonomyRows = autonomy.data?.data ?? [];

  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      eyebrow={eyebrow}
      title={t("ob.conv.prefs.title")}
      sub={t("ob.conv.prefs.body")}
    >
      <div className="ob-scene ob-prefs-scene">
        {/* An empty set is this seat's own answer — nothing is routed to it
            of any kind — and on this surface that is nothing to ask, not an
            empty card. */}
        {autonomyRows.length > 0 && (
          <section className="ob-prefs-section">
            <h3>{t("ob.conv.prefs.autonomyTitle")}</h3>
            <p className="ob-prefs-lead">{t("ob.conv.prefs.autonomyBody")}</p>
            <AutonomyChoices rows={autonomyRows} />
          </section>
        )}
        <WayOnward
          label={t("ob.conv.prefs.done")}
          pending={finishing}
          stillNeeded={(why) => why.join(" ")}
          note={
            finishFailed ? (
              <p className="ob-stage-note" role="alert">
                {t("ob.conv.prefs.persistFailed")}
              </p>
            ) : undefined
          }
          onGo={() => void finish()}
        />
      </div>
    </ConversationWorkbench>
  );
}
