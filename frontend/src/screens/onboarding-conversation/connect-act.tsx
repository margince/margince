import type { Dispatch } from "react";
import { useEffect, useState } from "react";
import { navigate } from "../../app/router";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import { problemMessageOf } from "../common";
import { EMPTY_DRAFT } from "../onboarding";
import { BuildScene } from "../onboarding-build-scene";
import {
  clearOAuthAttempt,
  ImapConnectPanel,
  OAuthConnectPanel,
  type OAuthProvider,
  OAuthReturnPanel,
  peekOAuthAttempt,
} from "../onboarding-connect-panels";
import {
  forgetOvernightChoice,
  rememberedOvernightChoice,
  rememberOvernightChoice,
  useSetAgentGrant,
} from "../overnight-grant";
import type { MailProvider } from "./connect-scene";
import { ConnectScene } from "./connect-scene";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import { railStops } from "./rail";
import { useSaveLinkedInAccount } from "./use-linkedin-account";
import type { WizardPersistInput } from "./use-wizard-state";
import { ConversationWorkbench } from "./workbench";

// The connect act: per-purpose consent as a conversation turn, provider
// cards that open their OWN dialog on the artifact surface (never an inline
// panel growing under the card, never a chip in the rail), and the finish
// gate. Finishing is a server fact before it is a UI fact: the completion
// checkpoint (step complete, connect skipped or not) must land before any
// navigation; a failed write is said out loud and retryable.
//
// The four step-level consent guarantees, and each provider's own
// disclosure, live entirely on `ConnectScene` and inside its dialogs now —
// the rail keeps only the two lines that are genuinely its own: what this
// step is for, and that connecting is optional per-provider even though a
// mailbox is required.

type ConnectActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  persist: (input: WizardPersistInput) => Promise<boolean>;
  /** The OAuth consent return's outcome from the deep-link route. */
  outcome?: string;
  /** The provider that consent returned for, from the same route. */
  returningProvider?: string;
}>;

// `outcome` and `returningProvider` are ROUTE segments — a stale bookmark or
// a reload of an old return URL replays them with no live attempt behind
// them. `peekOAuthAttempt` is the one thing that DOES prove this tab started
// the trip (see onboarding-connect-panels.tsx), so the reader only lands
// back inside the dialog it left from when the mark actually matches; a
// mismatch or an unmarked outcome falls back to the plain inline result
// instead of implying a return this tab never made.
function attemptedProvider(
  outcome: string | undefined,
  returningProvider: string | undefined,
): MailProvider | null {
  if (outcome === undefined) {
    return null;
  }
  const marked = peekOAuthAttempt();
  if (marked === null) {
    return null;
  }
  if (returningProvider !== undefined && returningProvider !== marked) {
    return null;
  }
  const byMarked: Record<OAuthProvider, MailProvider> = {
    gmail: "google",
    graph: "microsoft",
  };
  return byMarked[marked];
}

export function ConnectAct({
  state,
  dispatch,
  persist,
  outcome,
  returningProvider,
}: ConnectActProps) {
  const t = useT();
  // Which dialog is open. On an ordinary visit this starts null; on the real
  // page load a redirect lands on, it opens straight to the provider this
  // tab's own attempt marks — the reader lands back inside the same chrome
  // they left, rather than a bare surface with the result buried inline.
  const [provider, setProvider] = useState<MailProvider | null>(() =>
    attemptedProvider(outcome, returningProvider),
  );
  // Whether the OPEN dialog is showing that reopened result rather than a
  // fresh ask — cleared the moment the reader closes it or picks a
  // different provider, so a retry after a denied/failed return is a real
  // ask again, not a replay of the same result forever.
  const [resultFor, setResultFor] = useState<MailProvider | null>(() =>
    attemptedProvider(outcome, returningProvider),
  );
  const [finishing, setFinishing] = useState(false);
  const [finishFailed, setFinishFailed] = useState(false);
  // The overnight answer could not be recorded. The step still completes — see
  // finish() — so this is a notice, not a blocker.
  const [overnightFailed, setOvernightFailed] = useState(false);
  const [entering, setEntering] = useState(false);
  // Whether a returning OAuth trip has an actually-confirmed live mailbox,
  // told to us by `OAuthReturnPanel` itself (see `showSkip` below): false
  // for every unconfirmed state, including one this panel's own "enter"
  // fallback would otherwise let a reader click past.
  const [mailConfirmed, setMailConfirmed] = useState(false);
  // Whether the open dialog's own credential POST (OAuth's authorize call,
  // IMAP's connect) is still in flight, told to us by the panel itself. A
  // success landing after the reader already backed out via the dialog's X,
  // Escape, or backdrop would leave a mailbox connected against a "no" the
  // panel already promised, so `closeDialog` below refuses to act while true.
  const [dialogBusy, setDialogBusy] = useState(false);
  const linkedin = useSaveLinkedInAccount();

  // Spends the mark once this mount has read it, so reloading the same
  // return URL finds nothing and correctly falls back to the plain inline
  // result instead of reopening the dialog a second time.
  useEffect(() => {
    if (outcome !== undefined) {
      clearOAuthAttempt();
    }
  }, [outcome]);

  const openAsk = (key: MailProvider) => {
    setResultFor(null);
    setProvider(key);
  };
  const closeDialog = () => {
    if (dialogBusy) {
      return;
    }
    setProvider(null);
    setResultFor(null);
  };

  // The act advances LinkedIn only once the answer is STORED. Dispatching
  // first and saving in the background would let a member finish onboarding
  // believing they had connected, with nothing recorded and nothing to
  // correct. Unlike mail, resolving LinkedIn never touches `finish` — it is
  // a card on the same screen, not a gate on it.
  const connectLinkedin = (profileUrl: string) => {
    linkedin.mutate(
      { profileUrl, connected: true },
      {
        onSuccess: () =>
          dispatch({ type: "LINKEDIN_CONNECTED", profile: profileUrl }),
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

  const recordOvernightThenEnter = async () => {
    await recordOvernightAnswer();
    setEntering(true);
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
    // earlier session) recorded, so finishing can never overwrite a built
    // voice as skipped.
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
    // Completion is recorded, so the handoff can take its beat: the build scene
    // navigates when it is done. It resolves immediately under reduced motion,
    // so nobody is held behind an animation they asked not to see.
    setEntering(true);
  };

  if (entering) {
    return <BuildScene onDone={() => navigate({ screen: "home" })} />;
  }

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
          provider={provider}
          onPick={openAsk}
          onDialogClose={closeDialog}
          // True only for the ONE dialog this mount reopened from a proven
          // attempt (see `attemptedProvider`): its content is the settled
          // result, not a fresh ask, so the dialog's own chrome must say so.
          dialogShowsResult={provider !== null && provider === resultFor}
          onSkip={() => void finish(true)}
          wantsOvernight={wantsOvernight}
          onWantsOvernightChange={chooseOvernight}
          overnightFailed={overnightFailed}
          skipDisabled={finishing}
          // Once a mailbox is actually CONFIRMED live, "skip connecting" is
          // no longer a true option and recording the step as skipped would
          // persist a fact that is not so. Short of that confirmation —
          // still verifying, denied, unresolved, or verified absent — the
          // honest exit stays open: `OAuthReturnPanel`'s own "enter" button
          // in those states is a fallback the reader can also reach, but it
          // must never be the ONLY way out of an unconfirmed return.
          showSkip={outcome !== "ok" || !mailConfirmed}
          linkedinStatus={state.linkedinStatus}
          onLinkedinConnect={connectLinkedin}
          onLinkedinSkip={() => dispatch({ type: "LINKEDIN_SKIPPED" })}
          linkedinPending={linkedin.isPending}
          linkedinError={
            linkedin.isError ? problemMessageOf(linkedin.error, t) : null
          }
          // Entering from cn.done is a completion too, and the overnight answer
          // has to be recorded on THIS path as well: the step is already
          // persisted by the time the reader gets here (the connect panel's
          // own onComplete ran finish), so the answer is all that is left, and
          // routing it through finish again would re-persist the step.
          onEnter={
            state.phase === "cn.done"
              ? () => void recordOvernightThenEnter()
              : undefined
          }
          // The ask, still open: rendered INSIDE the dialog ConnectScene
          // wraps around `provider`. A real OAuth "allow" leaves the page
          // entirely (`location.assign`), so the dialog does not try to
          // stay open across that redirect — it simply closes on `onDismiss`
          // if the reader backs out first.
          dialogPanel={
            <>
              {provider === "google" && (
                <OAuthConnectPanel
                  provider="gmail"
                  onDismiss={closeDialog}
                  onPendingChange={setDialogBusy}
                />
              )}
              {provider === "microsoft" && (
                <OAuthConnectPanel
                  provider="graph"
                  onDismiss={closeDialog}
                  onPendingChange={setDialogBusy}
                />
              )}
              {provider === "imap" && (
                <ImapConnectPanel
                  onComplete={finish}
                  onDismiss={closeDialog}
                  onPendingChange={setDialogBusy}
                />
              )}
            </>
          }
          // The ask is settled: a redirect already returned. This is a
          // finding plus one more real decision (the backfill window), not a
          // fresh consent round — `ConnectScene` shows it in the dialog the
          // reader left from when a proven attempt says so, and inline on
          // the surface otherwise (a stale or bookmarked return link, which
          // is real information but not a return this tab can vouch for).
          returnPanel={
            outcome !== undefined ? (
              // How far back the first import reaches is `OnboardingBackread`'s
              // own question, rendered inside this panel once a mailbox is
              // confirmed live — the one history-read decision on this
              // surface, not a second one stacked beside it.
              <OAuthReturnPanel
                outcome={outcome}
                provider={returningProvider}
                onComplete={finish}
                onConfirmedChange={setMailConfirmed}
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
        <p className="ob-conv-notice" role="alert">
          {t("ob.conv.connect.persistFailed")}
        </p>
      )}
    </ConversationWorkbench>
  );
}
