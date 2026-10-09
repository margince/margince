import type { Dispatch } from "react";
import { useCallback, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { useLocale, useT } from "../../i18n";
import { useMe } from "../common";
import { InstallationSetup } from "../installation-setup";
import { OnboardingGate } from "../onboarding-gate";
import { CompanyActArtifact } from "./artifact";
import {
  useArtifactNavigation,
  useFindingHighlight,
  useProposalJoin,
} from "./company-act-state";
import { missingRequiredFields } from "./company-proposal";
import { CompanyDecision, CompanyReviewScene } from "./company-review-scene";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { presenceFor } from "./presence";
import type { ClarifyFailure } from "./use-clarify-answers";
import { useClarifyAnswers } from "./use-clarify-answers";
import { useCompanyConfirm } from "./use-company-confirm";
import { useCompanyDraft } from "./use-company-draft";
import { useCompanyGate } from "./use-company-gate";
import { safeStartError, useCompanyRead } from "./use-company-read";
import { useReviewRows } from "./use-review-rows";
import type { WizardPersistInput } from "./use-wizard-state";
import { ConversationWorkbench, useConfiguredModel } from "./workbench";

// The company act driver. The read lifecycle lives in useCompanyRead, clarify
// authorization in useClarifyAnswers and the confirmation in useCompanyConfirm.
// Every step is a machine event, so the reducer stays the one truth about
// where the conversation is. The rail takes no free text: a pick answers on
// its DecisionScene and a field is typed on the review surface.

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];
type LegalEntity = components["schemas"]["CompanySiteReadLegalEntity"];
type AiRuntime = NonNullable<CompanySiteRead["ai_runtime"]>;

type CompanyActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  /** The member path's existing company; the draft seeds from it so a
   * confirmation can never erase stored fields the read did not rediscover. */
  profile: CompanyProfile | null;
  persist: (input: WizardPersistInput) => Promise<boolean>;
  /** The restored snapshot of the machine's already-active read (reload
   * adoption); null in a live session. */
  adoptedRead?: CompanySiteRead | null;
}>;

export function CompanyAct({
  state,
  dispatch,
  profile,
  persist,
  adoptedRead = null,
}: CompanyActProps) {
  const t = useT();
  const { locale } = useLocale();
  // The gate greets by the whole display_name: name order is not universal,
  // so a first token would greet some contacts by their family name.
  const me = useMe();
  const configuredModel = useConfiguredModel();
  const { draft, draftRef, setDraft, applyChanges, setField, pickEntity } =
    useCompanyDraft(profile);
  const [selectedFactKeys, setSelectedFactKeys] = useState<string[]>([]);
  const nav = useArtifactNavigation();
  const { proposalJoin, onReadStarted } = useProposalJoin(
    state,
    persist,
    draftRef,
  );
  const machine = useRef(state);
  machine.current = state;

  const proposalRef = useRef<Proposal | undefined>(undefined);
  // The read's own candidates for the legal-entity fill, kept current once
  // `read` is computed below.
  const legalEntitiesRef = useRef<readonly LegalEntity[]>([]);
  const clarify = useClarifyAnswers({
    locale,
    proposalRef,
    draftRef,
    legalEntitiesRef,
    // No chat transcript: each clarify authorization stands on its own terms.
    history: () => [],
    applyChanges,
    applyLegalEntity: pickEntity,
  });

  const { startRead, siteRead, proposal, prevSnapshot } = useCompanyRead({
    dispatch,
    machine,
    setDraft,
    setSelectedFactKeys,
    answers: clarify.answers,
    onReadStarted,
    proposalJoin,
    adoptedRead,
  });
  proposalRef.current = proposal.data;

  // Picking a legal entity also retires the sibling questions it settles, but
  // only the authorization can know the choice was confirmed
  // (retireLegalSiblings in use-clarify-answers.ts).
  const handleAnswer = useCallback(
    (questionId: string, value: string) => {
      dispatch({ type: "QUESTION_ANSWERED", questionId, value });
      clarify.answerClarify(questionId, value);
    },
    [dispatch, clarify.answerClarify],
  );
  // Humans outrank the reader: a dismissal clears the pending question through
  // the ordinary answer path and stops it counting as an open decision.
  const handleDismiss = useCallback(
    (questionId: string) => {
      dispatch({
        type: "QUESTION_ANSWERED",
        questionId,
        value: "",
        dismissed: true,
      });
      clarify.dismissClarify(questionId);
    },
    [dispatch, clarify.dismissClarify],
  );

  const { confirm, ...confirmState } = useCompanyConfirm({
    dispatch,
    persist,
    draftRef,
    selectedFactKeys,
    prevSnapshot,
    siteRead,
    proposal,
    answers: clarify.answers,
    t,
  });

  const read = siteRead.data ?? startRead.data ?? null;
  legalEntitiesRef.current = read?.legal_entities ?? [];
  const missing = missingRequiredFields(draft.values);
  const readBroken = startRead.isError || siteRead.isError;
  const review = useReviewRows({
    proposal: proposal.data,
    read,
    draft,
    pendingId: state.pendingQuestion?.id ?? null,
    answers: clarify.answers,
    t,
  });
  const runtime = latestRuntime(read?.ai_runtime, clarify.runtime);
  const highlight = useFindingHighlight(state);
  const presence = presenceFor(state, { read, readBroken });
  const gate = useCompanyGate({
    state,
    dispatch,
    setDraft,
    startRead,
    read,
    locale,
    t,
  });

  if (gate.setupOutstanding) {
    return <InstallationSetup />;
  }
  // One return for both faces of the gate, because they are one column: two
  // returns would remount everything between them (see GateColumn).
  if (gate.beforeReview) {
    return (
      <OnboardingGate
        name={me.data?.user.display_name}
        running={gate.running}
        notice={gate.notice}
        configuredModel={configuredModel}
        scan={gate.scan}
        readBroken={readBroken}
        uncertainCount={review.uncertainCount}
        onSubmit={gate.onSubmit}
        onManual={() => dispatch({ type: "MANUAL_CHOSEN" })}
        onRetryRead={() => siteRead.refetch()}
      />
    );
  }

  const { held: confirmHeld, refused: confirmRefused } = confirmHold(
    state,
    confirmState.blocked,
    missing,
  );
  const runConfirm = () => confirm.mutate();

  // One scene on the surface at a time. A pending decision owns it, the review
  // owns it after, and the rail carries narration and history.
  const decision =
    state.phase === "co.clarify" && state.pendingQuestion !== null ? (
      <CompanyDecision
        question={state.pendingQuestion}
        read={read}
        onAnswer={handleAnswer}
        onDismiss={handleDismiss}
      />
    ) : null;
  const reviewScene =
    state.phase === "co.review" && review.reviewProposal ? (
      <CompanyReviewScene
        mode={nav.artifactMode}
        onSwitchMode={nav.setArtifactMode}
        onReadWhole={nav.readWhole}
        onSettle={nav.settleField}
        goToField={nav.goToField}
        proposal={review.reviewProposal}
        read={read}
        companyId={profile?.company_id}
        draft={draft}
        setField={setField}
        rows={{
          all: review.allRows,
          cards: review.cards,
          cardOf: review.cardOf,
          openQuestions: review.openQuestionCount,
        }}
        answers={clarify.answers}
        missing={missing}
        selectedFactKeys={selectedFactKeys}
        setSelectedFactKeys={setSelectedFactKeys}
        confirm={{
          run: runConfirm,
          pending: confirm.isPending,
          blockers: review.confirmBlockers,
          held: confirmHeld,
        }}
        authorizing={clarify.authorizing || confirmState.blocked}
        error={confirmState.bannerMessage}
      />
    ) : null;
  return (
    <ConversationWorkbench
      core={presence.core}
      progress={presence.progress}
      railState={state}
      status={readStatus(readBroken, read, t)}
      runtime={runtime}
      {...boardHeading(state, t)}
    >
      {
        <CompanyActArtifact
          mode={nav.artifactMode}
          manual={state.phase === "co.manual"}
          review={decision ?? reviewScene}
          read={read}
          draft={draft}
          setField={setField}
          onPickEntity={pickEntity}
          selectedFactKeys={selectedFactKeys}
          setSelectedFactKeys={setSelectedFactKeys}
          missingRequired={missing}
          highlight={highlight}
          onSwitchMode={nav.setArtifactMode}
          onConfirm={runConfirm}
          confirmPending={confirm.isPending}
          confirmDisabled={confirmRefused}
          saveError={confirmState.bannerMessage}
          refusal={confirmState.refusal}
        />
      }
      <ActFailures
        startError={
          startRead.isError ? safeStartError(startRead.error, t) : null
        }
        clarifyFailure={clarify.failure}
      />
    </ConversationWorkbench>
  );
}

/**
 * Whether a confirm would be refused. The deck and the whole-profile card both
 * ask, so it is spelled once.
 *
 * Every term is a reason the server would refuse. A required field is empty,
 * the phase has nothing to confirm, or the server already refused in a way
 * pressing again cannot change. `held` leaves out the empty fields, which the
 * rail names when Confirm is pressed early.
 */
function confirmHold(
  state: ConversationState,
  blocked: boolean,
  missing: readonly unknown[],
): Readonly<{ held: boolean; refused: boolean }> {
  const held =
    blocked || !(state.phase === "co.review" || state.phase === "co.manual");
  return { held, refused: missing.length > 0 || held };
}

// The runtime bar keeps the live read total unless a clarify authorization saw
// more calls. Answering a decision is a model round trip too.
function latestRuntime(
  readRuntime: AiRuntime | undefined,
  clarifyRuntime: AiRuntime | undefined,
): AiRuntime | undefined {
  return clarifyRuntime &&
    (!readRuntime || clarifyRuntime.call_attempts >= readRuntime.call_attempts)
    ? clarifyRuntime
    : readRuntime;
}

function readStatus(
  readBroken: boolean,
  read: CompanySiteRead | null,
  t: ReturnType<typeof useT>,
): string {
  if (readBroken) {
    return t("ob.readStatus.failed");
  }
  return read ? t(`ob.readStatus.${read.status}`) : t("ob.ai.ready");
}

// The failures that need a retry. What still wants an answer is not listed
// here: the deck is that list, counted in its own tray.
function ActFailures({
  startError,
  clarifyFailure,
}: Readonly<{
  startError: string | null;
  clarifyFailure: ClarifyFailure | null;
}>) {
  const t = useT();
  return (
    <>
      {startError !== null && (
        <ErrorLine>
          {t("ob.gate.startFailed", { detail: startError })}
        </ErrorLine>
      )}
      {clarifyFailure && (
        <ErrorLine>
          {clarifyFailure.kind === "request"
            ? t("ob.conv.clarify.applyFailed", {
                detail: clarifyFailure.detail,
              })
            : t("ob.conv.clarify.applyMissing")}
        </ErrorLine>
      )}
    </>
  );
}

/**
 * What the room says this screen is, per phase.
 *
 * The question is the title while one is pending. A standing heading with the
 * question repeated in a card below would say it twice and push the cards off
 * the fold. `DecisionScene` labels its options by pointing at this heading.
 */
function boardHeading(
  state: ConversationState,
  t: ReturnType<typeof useT>,
): Readonly<{ eyebrow?: string; title: string; sub?: string }> {
  if (state.phase === "co.clarify" && state.pendingQuestion !== null) {
    return {
      eyebrow: t("ob.conv.scene.settleEyebrow"),
      title: t(state.pendingQuestion.i18nKey, state.pendingQuestion.params),
      sub: t("ob.conv.scene.decisionSub"),
    };
  }
  if (state.phase === "co.manual") {
    return { title: t("ob.conv.manual.boardTitle") };
  }
  return {
    eyebrow: t("ob.deck.eyebrow"),
    title: t("ob.deck.title"),
    sub: t("ob.conv.review.boardSub"),
  };
}
