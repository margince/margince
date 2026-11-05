import type { Dispatch } from "react";
import { useEffect, useState } from "react";
import { ErrorLine } from "../../../design-system/errorline";
import { vendorOf } from "../../../format/connectorprovider";
import { ordinalNumber } from "../../../format/format";
import { useT } from "../../../i18n/index";
import {
  forgetOvernightChoice,
  rememberedOvernightChoice,
  rememberOvernightChoice,
  useSetAgentGrant,
} from "../../../lib/overnight-grant";
import { problemMessageOf } from "../../../lib/problem";
import { navigateReplacing } from "../../../lib/router";
import { useSaveLinkedInAccount } from "../../../lib/use-linkedin-account";
import { EMPTY_DRAFT } from "../onboarding";
import {
  clearOAuthAttempt,
  OAuthReturnPanel,
  peekOAuthAttempt,
} from "../onboarding-connect-panels";
import { ConnectScene } from "./connect-scene";
import type { MailProvider } from "./connect-vendor";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import type { WizardPersistInput } from "./use-wizard-state";
import { ConversationWorkbench } from "./workbench";

// The connect act: the work account picked once and asked for inline, mail
// and calendar in one press, and the way on. A connected mailbox leaves the
// reader on the step, LinkedIn still waiting beside it.
// Leaving is a server fact before it is a UI fact. How the step was left
// (a mailbox connected, or none) must land before the act moves on. A failed
// write is said out loud and retryable.

type ConnectActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
  /** The OAuth consent return's outcome from the deep-link route. */
  outcome?: string;
  /** The provider that consent returned for, from the same route. */
  returningProvider?: string;
}>;

// `outcome` and `returningProvider` are route segments: a stale bookmark or a
// reload of an old return URL replays them with no live attempt behind them.
// Only `peekOAuthAttempt` proves this tab started the trip. So the account a
// refused trip was for comes back picked only when the mark and the route name
// the same vendor. A mail-and-calendar trip marks the mailbox and lands on the
// calendar, which is the same account.
function returnedVendor(
  outcome: string | undefined,
  returningProvider: string | undefined,
): MailProvider | null {
  // A granted mailbox needs no ask reopened; the return says what came back.
  if (outcome === undefined || outcome === "ok") {
    return null;
  }
  const marked = vendorOf(peekOAuthAttempt() ?? undefined);
  if (marked === null) {
    return null;
  }
  if (
    returningProvider !== undefined &&
    vendorOf(returningProvider) !== marked
  ) {
    return null;
  }
  return marked;
}

export function ConnectAct({
  state,
  dispatch,
  persist,
  outcome,
  returningProvider,
}: ConnectActProps) {
  const t = useT();
  // The account whose ask is open. On the page load a proven redirect lands on,
  // it is the account that trip was for. A refused consent can then be asked
  // again from where the reader left.
  const [vendor, setVendor] = useState<MailProvider | null>(() =>
    returnedVendor(outcome, returningProvider),
  );
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  // The overnight answer could not be recorded. The step still completes (see
  // finish()), so this is a notice, not a blocker.
  const [overnightFailed, setOvernightFailed] = useState(false);
  // Whether the open ask's own credential POST is still in flight. The scene
  // holds every other way off the ask while it is.
  const [askBusy, setAskBusy] = useState(false);
  // The consent return's result has been closed: it renders nowhere after
  // that, whatever the address still says.
  const [resultSettled, setResultSettled] = useState(false);
  const linkedin = useSaveLinkedInAccount();

  // Spends the mark once this mount has read it, so reloading the same
  // return URL reads as the stale link it then is.
  useEffect(() => {
    if (outcome !== undefined) {
      clearOAuthAttempt();
    }
  }, [outcome]);

  // A result whose mailbox question is answered (a consent returned, an IMAP
  // host connected) closes onto the step. Its roster now shows the account as
  // connected. The outcome in the address goes with it. Left there, a reload
  // would show a trip this tab already finished.
  const settleResult = () => {
    setVendor(null);
    setResultSettled(true);
    if (outcome !== undefined) {
      navigateReplacing({ screen: "onboarding", id: "company" });
    }
  };

  // The act records LinkedIn only once the answer is STORED. Dispatching first
  // and saving in the background would let a member finish onboarding
  // believing their profile was on file, with nothing recorded and nothing to
  // correct. Saving a profile is not authorizing anything: `connected` stays
  // false, and the store keeps whatever authorization already exists. Unlike
  // mail, resolving LinkedIn never touches `finish` — it is a card on the same
  // screen, not a gate on it.
  const saveLinkedin = (profileUrl: string) => {
    linkedin.mutate(
      { profileUrl, connected: false },
      {
        onSuccess: () =>
          dispatch({ type: "LINKEDIN_SAVED", profile: profileUrl }),
      },
    );
  };

  // Preselected: the features it feeds are the ones the product opens on, so
  // an unticked default ships an installation whose morning brief is
  // permanently empty for reasons nobody is told.
  //
  // Seeded from this tab's remembered answer, because an OAuth "allow" leaves
  // the page and this component remounts on the way back. Without the seed a
  // rep who UNTICKED the box, authorized Google, and returned would be granted
  // anyway — their opt-out silently reversed by a default. `undefined` means
  // this tab holds no answer, which is when the default is genuinely the
  // default rather than an override.
  const [wantsOvernight, setWantsOvernight] = useState(
    () => rememberedOvernightChoice() ?? true,
  );
  const grantOvernight = useSetAgentGrant();
  const chooseOvernight = (next: boolean) => {
    setWantsOvernight(next);
    rememberOvernightChoice(next);
  };

  // The answer, recorded once, whichever way the reader leaves the step.
  //
  // It is idempotent at the server (re-answering replaces), and the local mark
  // is dropped after it lands so a second pass through this screen in the same
  // tab starts from the default rather than from a settled answer.
  const recordOvernightAnswer = async () => {
    try {
      await grantOvernight.mutateAsync(wantsOvernight);
      forgetOvernightChoice();
    } catch {
      setOvernightFailed(true);
    }
  };

  const finish = async (skipped: boolean) => {
    setFinishing(true);
    setFinishFailed(false);
    // The overnight answer rides with the step rather than being written when
    // the box is ticked: it is preselected, so writing on tick would grant an
    // authority for every reader who merely passed through this screen. Here it
    // is recorded only by a rep who actually completed the step.
    //
    // BOTH answers are recorded, not only the yes. A decline is a real answer
    // the product needs stored: unanswered and declined are different states,
    // and leaving an opt-out as unanswered is what makes the product ask the
    // declining rep again every night.
    //
    // A rep who SKIPS the connect records nothing, whatever the box says — the
    // agent reads their mail to build the brief, so an answer about a mailbox
    // that was never connected is an answer about nothing.
    //
    // Its failure does not fail the step, but it is not swallowed either: the
    // rep is told, and the question is askable again in Settings. Blocking
    // onboarding on it would trade a recoverable gap for an unrecoverable one;
    // saying nothing would leave them believing they answered.
    if (!skipped) {
      await recordOvernightAnswer();
    }
    // Voice flags are NOT sent: the merge keeps whatever the voice act (or an
    // earlier session) recorded, so leaving here can never overwrite a built
    // voice as skipped. Leaving IS finishing: this is the last stop, so the
    // step written is "complete", and a reload before it lands comes back
    // here, to a screen that already shows what is connected.
    const persisted = await persist({
      step: "complete",
      values: EMPTY_DRAFT.values,
      connectSkipped: skipped,
    });
    setFinishing(false);
    if (!persisted) {
      setFinishFailed(true);
      return;
    }
    dispatch({ type: "CONNECT_DONE" });
  };

  // Where the journey stands, in the rail's own counting.
  const stops = railStops(state.memberPath);
  const eyebrow = t("ob.conv.scene.step", {
    n: ordinalNumber(stops.findIndex((stop) => stop.key === "connect") + 1),
    m: ordinalNumber(stops.length),
    label: t("ob.rail.connect"),
  });
  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      eyebrow={eyebrow}
      title={t("ob.conv.connect.sceneTitle")}
      sub={t("ob.conv.connect.sceneSub")}
    >
      {
        <ConnectScene
          vendor={vendor}
          onVendorChange={setVendor}
          askBusy={askBusy}
          onAskBusyChange={setAskBusy}
          onImapDone={settleResult}
          onFinish={(skipped) => void finish(skipped)}
          finishing={finishing}
          wantsOvernight={wantsOvernight}
          onWantsOvernightChange={chooseOvernight}
          overnightFailed={overnightFailed}
          linkedinStatus={state.linkedinStatus}
          onLinkedinSave={saveLinkedin}
          onLinkedinSkip={() => dispatch({ type: "LINKEDIN_SKIPPED" })}
          linkedinPending={linkedin.isPending}
          linkedinError={
            linkedin.isError ? problemMessageOf(linkedin.error, t) : null
          }
          // A redirect already returned: what it could not do, shown inline whether or
          // not this tab can vouch for the trip. A stale link still reports what the
          // roster says.
          returnPanel={
            outcome !== undefined && !resultSettled ? (
              <OAuthReturnPanel
                outcome={outcome}
                provider={returningProvider}
                onDone={settleResult}
              />
            ) : null
          }
        />
      }
      {/* The consent narration and the picker prompt are said once, on the
          scene itself now (the guarantees grid, each provider's own
          disclosure). What survives from the thread is the one thing a
          reader still has to act on: a finish that could not be recorded. */}
      {finishFailed && (
        <ErrorLine>{t("ob.conv.connect.persistFailed")}</ErrorLine>
      )}
    </ConversationWorkbench>
  );
}
