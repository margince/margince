import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { Dispatch, SetStateAction } from "react";
import { useCallback, useMemo, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  problemCodeOf,
  problemMessageOf,
  throwProblem,
  useMe,
} from "../common";
import {
  InstallationSetup,
  outstandingStep,
  useInstallationSetup,
  usePlatformDeclined,
} from "../installation-setup";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import {
  changeDraftField,
  EMPTY_DRAFT,
  formFromProfile,
  isRequired,
  normalizeUrl,
} from "../onboarding";
import { OnboardingGate } from "../onboarding-gate";
import type { SuggestedCompanyChange } from "../onboarding-read";
import type {
  ArtifactMode,
  ConfirmRefusal,
  FindingHighlight,
  RefusalRetry,
} from "./artifact";
import { CompanyActArtifact } from "./artifact";
import {
  draftWithLegalEntity,
  evidencedFields,
  isCompanyField,
  legalEntityForOption,
  missingRequiredFields,
  proposalFromRead,
  resolutionsFromAnswers,
} from "./company-proposal";
import {
  isWork,
  type ReviewRow,
  reviewFields,
  rowFor,
} from "./company-review-state";
import { CompanyConfirmCard } from "./confirm-card";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { DecisionScene } from "./decision-scene";
import { gateNoticeFor } from "./gate-notice";
import { presenceFor } from "./presence";
import { ProfileDigest } from "./profile-digest";
import { deckCards, ReviewDeck } from "./review-deck";
import { useClarifyAnswers } from "./use-clarify-answers";
import { safeStartError, useCompanyRead } from "./use-company-read";
import type { WizardPersistInput } from "./use-wizard-state";
import { WayOnward } from "./way-onward";
import { ConversationWorkbench, useConfiguredModel } from "./workbench";

// The company act driver: the read lifecycle lives in useCompanyRead and
// clarify authorization in useClarifyAnswers; this component owns the draft
// and the one explicit confirmation — all expressed as machine events, so
// the pure reducer stays the single truth about where the conversation is.
// The rail takes no free text: a legal-entity pick answers on its own
// DecisionScene, a field is typed on the review surface, and every other
// reply the human can give is one of the rail's own chips or jump links —
// never a composer.

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];
type LegalEntity = components["schemas"]["CompanySiteReadLegalEntity"];

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

function initialDraft(profile: CompanyProfile | null): CompanyDraft {
  return profile
    ? { values: formFromProfile(profile), grounded: {}, edited: new Set() }
    : EMPTY_DRAFT;
}

// Which server state a confirm 409 named — see the state declaration in the
// driver for what each one means to the reader.
type ConfirmNotice = "skew" | "notReady" | "checkFailed";

// Whether the version pair the next confirm would quote is the one this read
// carries — the whole question a readiness re-check has to answer about the
// proposal half, since the pair is all the confirm takes from it.
//
// The proposal is that pair's source whenever it has one: `ready` is the
// server's own word for "this read has a draft to confirm", and the pair has
// to be THIS read's, not one from before it moved. A proposal endpoint that
// has never answered is the one case that needs no comparison — the confirm
// then quotes the refreshed read's own pair (see the mutation body's
// `proposalFromRead` fallback) — and it is distinguishable from a stale pair
// precisely because no snapshot was ever kept to go stale.
function quotesReadVersion(
  quoted: Proposal | undefined,
  refreshed: CompanySiteRead,
): boolean {
  if (quoted === undefined) {
    return true;
  }
  return (
    quoted.ready &&
    quoted.draft_version === refreshed.draft_version &&
    quoted.proposal_hash === refreshed.proposal_hash
  );
}

// The sentence a notice reads as. A "skew" has two, because the reader's
// next step changes once its refresh has run and the block is still there.
function confirmNoticeKey(
  notice: ConfirmNotice,
  skewStuck: boolean,
): MessageKey {
  if (notice === "notReady") {
    return "ob.conv.review.confirmNotReady";
  }
  if (notice === "checkFailed") {
    return "ob.conv.review.confirmCheckFailed";
  }
  return skewStuck
    ? "ob.conv.review.confirmVersionSkewStuck"
    : "ob.conv.review.confirmVersionSkew";
}

// biome-ignore lint/complexity/noExcessiveCognitiveComplexity: the act driver is one machine-shaped surface; splitting it further would scatter the event wiring
export function CompanyAct({
  state,
  dispatch,
  profile,
  persist,
  adoptedRead = null,
}: CompanyActProps) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  // The gate greets by name, and uses the whole display_name rather than a
  // first token: name order is not universal, so slicing one off would greet
  // some people by their family name.
  const me = useMe();
  const configuredModel = useConfiguredModel();

  // Draft state mirrors the classic coordinator: values + grounding +
  // human-edited marks move together, and a ref keeps callbacks current.
  const [draft, setDraftState] = useState<CompanyDraft>(() =>
    initialDraft(profile),
  );
  const draftRef = useRef<CompanyDraft>(draft);
  const setDraft = useCallback((update: SetStateAction<CompanyDraft>) => {
    const next =
      typeof update === "function" ? update(draftRef.current) : update;
    draftRef.current = next;
    setDraftState(next);
  }, []);

  const [selectedFactKeys, setSelectedFactKeys] = useState<string[]>([]);
  const [artifactMode, setArtifactMode] = useState<ArtifactMode>("dossier");
  // Which field the deck should open on next, set by the whole-record
  // document's "Settle it" pill and read once by the deck it switches back
  // to. Cleared whenever the reader reaches the document any other way, so a
  // stale settle target from minutes ago cannot reach back into a deck the
  // reader has since moved past on their own.
  const [goToField, setGoToField] = useState<CompanyFieldName | null>(null);
  const settleField = useCallback((field: CompanyFieldName) => {
    setGoToField(field);
    setArtifactMode("dossier");
  }, []);
  // A confirm 409 that this driver has already resolved into a next step,
  // rather than a bare failure. The server gives each of its three refusals
  // its own code (crm.yaml, confirmCompanySiteRead), so each notice states
  // what the server itself said: "skew" is `version_skew`, the draft changed
  // under the human; "notReady" is `not_confirmable`, this read has no draft
  // to confirm; and "checkFailed" is the one thing left unresolved after an
  // `already_confirmed` — the company that confirmation created could not be
  // loaded, an honest "I could not load it", never a diagnosis of the read
  // stated on the strength of a request that never landed. None is the
  // generic confirmFailed banner — see confirmBannerMessage below — because
  // none is "fix your input and try the same thing again". This is the
  // banner's OWN category and outlives the refetch below: it clears only at
  // the next confirm attempt (onMutate) or at the reader's own check that
  // ends the state it describes, never the moment Continue happens to
  // re-arm, so the reassurance stays on screen until the reader has had a
  // chance to read it.
  const [confirmNotice, setConfirmNotice] = useState<ConfirmNotice | null>(
    null,
  );
  // Whether a "skew" 409 blocks the NEXT press of Continue, settled in one
  // of three outcomes once its refetch (or the reader's own manual retry of
  // one) resolves — (1) a genuinely NEW proposal hash landed: the block
  // lifts, because the next press would send something the server has not
  // seen yet; (2) the refetch succeeded but the hash is UNCHANGED:
  // re-arming would earn the identical 409, so the block stays and
  // `skewStuck` switches the banner to naming that and offering a manual
  // retry; (3) the refetch itself FAILED: same outcome as (2), since there
  // is nothing new to resubmit either way.
  const [skewBlocked, setSkewBlocked] = useState(false);
  const [skewStuck, setSkewStuck] = useState(false);
  // True for as long as this driver's own refetch (automatic or a manual
  // retry of a "skewStuck" notice) is in flight — disables the retry action
  // itself, so a second press cannot start a second refetch on top of one
  // already running.
  const [awaitingProposalRefresh, setAwaitingProposalRefresh] = useState(false);
  // Whether a `not_confirmable` 409 blocks the NEXT press of Continue. It
  // always does when it lands: the server refused this read as having no
  // draft to confirm, and the identical submission earns the identical
  // refusal until the read itself moves. Only a re-check that finds the read
  // confirmable again lifts it — see recheckReadiness.
  const [notReadyBlocked, setNotReadyBlocked] = useState(false);
  // The same as awaitingProposalRefresh, for this driver's two other looks —
  // the readiness re-check and the confirmed-company load. While one is in
  // flight its own retry is disabled, so a second press cannot start a
  // second look on top of the one already running.
  const [awaitingReadinessCheck, setAwaitingReadinessCheck] = useState(false);
  const [awaitingCompanyLoad, setAwaitingCompanyLoad] = useState(false);
  // The proposal hash THIS confirm attempt submitted, captured inside the
  // mutation so the version-skew refresh below can tell "the refetch moved
  // past the rejected draft" from "the refetch landed the exact same one" —
  // the only distinction that decides whether the block lifts.
  const submittedProposalHashRef = useRef<string | undefined>(undefined);
  // A run the machine already owns at mount was persisted when it started
  // (that is how restore found it), so its wizard-state join is already in
  // place; a fresh session joins when its own read starts.
  const [proposalJoin, setProposalJoin] = useState<
    "pending" | "ready" | "failed"
  >(() => (state.activeReadId !== null ? "ready" : "pending"));
  const machine = useRef(state);
  machine.current = state;

  const applyChanges = useCallback(
    (changes: readonly SuggestedCompanyChange[]) => {
      setDraft((current) => {
        let next = current;
        for (const change of changes) {
          next = changeDraftField(next, change.field, change.value);
        }
        return next;
      });
    },
    [setDraft],
  );

  const proposalRef = useRef<Proposal | undefined>(undefined);
  // The read's own candidates, live for the legal-entity fill below — kept
  // current a few lines down, once `read` itself is computed.
  const legalEntitiesRef = useRef<readonly LegalEntity[]>([]);
  const clarify = useClarifyAnswers({
    locale,
    proposalRef,
    draftRef,
    legalEntitiesRef,
    // The rail takes no free text, so there is no chat transcript to send —
    // every clarify authorization is a fresh request on its own terms.
    history: () => [],
    applyChanges,
    // A whole-entity pick, unlike applyChanges: its provenance and its
    // never-overwrite-an-edit guard belong to the draft helper, not to a
    // loop over field/value pairs. The same helper the dossier's own entity
    // cards call, because it is the same gesture — see draftWithLegalEntity.
    applyLegalEntity: (entity) =>
      setDraft((current) => draftWithLegalEntity(current, entity)),
  });

  // The proposal endpoint joins through persisted wizard state, so the
  // running read is recorded the moment it starts — and the proposal fetch
  // waits for that write (a stale join would serve the previous read).
  const onReadStarted = useCallback(
    (started: CompanySiteRead) => {
      setProposalJoin("pending");
      void persist({
        step: "read",
        mode: "website",
        readId: started.id,
        values: draftRef.current.values,
      }).then((ok) => setProposalJoin(ok ? "ready" : "failed"));
    },
    [persist],
  );

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

  // Answering is the whole gesture from here. Picking a legal entity also
  // retires the sibling questions that pick settles, but that belongs to the
  // authorization rather than to the click — see retireLegalSiblings in
  // use-clarify-answers.ts, which is the only thing that can know the choice
  // was actually confirmed.
  const handleAnswer = useCallback(
    (questionId: string, value: string) => {
      dispatch({ type: "QUESTION_ANSWERED", questionId, value });
      clarify.answerClarify(questionId, value);
    },
    [dispatch, clarify.answerClarify],
  );

  // Humans outrank the reader: dismissing a clarify resolves it locally —
  // the machine's pending question clears through the ordinary answer path
  // and the recorded dismissal stops it counting as an open decision.
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

  // The one refresh a version-skew 409 ever runs, whether it fires
  // automatically (the mutation's own onError) or the reader asks for
  // another look at a "skewStuck" notice — one place, so the three-outcome
  // decision below can never drift between the two callers. The proposal is
  // cached separately from the read snapshot: refetching only the read
  // leaves the stale proposal in place, so both are awaited together.
  const refreshAfterSkew = useCallback(() => {
    setAwaitingProposalRefresh(true);
    const submittedHash = submittedProposalHashRef.current;
    void Promise.allSettled([siteRead.refetch(), proposal.refetch()]).then(
      ([, proposalOutcome]) => {
        setAwaitingProposalRefresh(false);
        const refreshedHash =
          proposalOutcome.status === "fulfilled"
            ? proposalOutcome.value.data?.proposal_hash
            : undefined;
        // Three outcomes, and only the first lifts the block: (1) the
        // refetch succeeded and came back with a DIFFERENT hash — the next
        // press provably sends a draft the server has not rejected; (2) the
        // refetch succeeded but the hash is UNCHANGED — resubmitting would
        // earn the identical 409, so the block stays and `skewStuck` tells
        // the reader so, with a route to ask again; (3) the refetch itself
        // FAILED — same outcome as (2), since there is nothing new to
        // resubmit. The banner's own text (confirmNotice) is untouched
        // either way: it already says "have a look, then press Continue
        // again", which stays true whether that press is available now or
        // needs one more look first.
        const landedNewDraft =
          refreshedHash !== undefined && refreshedHash !== submittedHash;
        setSkewBlocked(!landedNewDraft);
        setSkewStuck(!landedNewDraft);
      },
    );
  }, [siteRead.refetch, proposal.refetch]);

  // The one route off the review, whether this attempt confirmed it or an
  // earlier one already did (the recovered "already confirmed" 409 below
  // reaches this too) — one place, so the checkpoint and the machine
  // transition can never drift between the two callers.
  const finishConfirm = useCallback(
    (profileData: CompanyProfile) => {
      // The shell's onboarding gate reads the same ["company"] cache entry.
      queryClient.setQueryData(["company"], profileData);
      // Checkpoint the confirmed company so the classic coordinator resumes
      // at the right step and role if the user switches shells.
      void persist({
        step: "basis",
        mode: prevSnapshot.current !== null ? "website" : "manual",
        readId: prevSnapshot.current?.id ?? null,
        values: draftRef.current.values,
        factKeys: selectedFactKeys,
      });
      dispatch({ type: "COMPANY_CONFIRMED" });
    },
    [dispatch, persist, prevSnapshot, queryClient, selectedFactKeys],
  );

  // The one route an `already_confirmed` 409 takes, whether it fires from the
  // mutation's own onError or the reader asks to look again — one place, so
  // the outcomes below cannot drift between the two callers.
  //
  // The server has already named THIS read as confirmed, so there is nothing
  // left to diagnose: the only thing standing between the reader and the
  // review's exit is the company that confirmation created. Loading it is
  // therefore the whole recovery — no refetch of the read to re-derive which
  // 409 this was, and no reading of GET /company as proof of anything (the
  // member path can carry a company from before the attempt began; here the
  // server itself already answered that question).
  //
  // The load can fail, and a failed load is its own outcome rather than a
  // fall-through: leaving it silent would put the reader back on a Continue
  // button with nothing said, which is the loop this branch exists to close.
  const loadConfirmedCompany = useCallback(() => {
    setAwaitingCompanyLoad(true);
    const load = async () => {
      const { data } = await api.GET("/company");
      if (data === undefined) {
        setConfirmNotice("checkFailed");
        return;
      }
      finishConfirm(data);
    };
    void load()
      .catch((loadError) => {
        // Whatever actually broke belongs in the console, never in the
        // sentence the human reads — and never nowhere, which is what left
        // Continue re-armed onto the same rejection.
        console.error("confirmed-company load failed", loadError);
        setConfirmNotice("checkFailed");
      })
      .finally(() => setAwaitingCompanyLoad(false));
  }, [finishConfirm]);

  // The one look a `not_confirmable` 409 ever takes, and the ONLY thing that
  // lifts the block it raised. The server refused this read as having no
  // draft to confirm, so nothing about the same submission can end
  // differently until the read itself moves; only a snapshot the server now
  // reports as confirmable proves it has. A refetch that FAILED proves
  // nothing — react-query keeps the last good snapshot, which is the very
  // one the server just refused — so the block stands, exactly as it does
  // for a snapshot that comes back unconfirmable again.
  //
  // Both halves are checked, on their own terms, because the confirm sends
  // both: a read the server calls confirmable AND the version pair that press
  // would quote. A refetch that merely SETTLED proves nothing about the pair —
  // react-query serves its last good proposal through a failure, and a
  // successful one can still answer for a draft the read has since moved past
  // — so the block lifts only once that pair IS the refreshed read's own. Any
  // other release re-arms Continue onto a version_skew earned on the next
  // press rather than avoided.
  const recheckReadiness = useCallback(() => {
    setAwaitingReadinessCheck(true);
    void Promise.allSettled([siteRead.refetch(), proposal.refetch()]).then(
      ([readOutcome, proposalOutcome]) => {
        setAwaitingReadinessCheck(false);
        const refreshed =
          readOutcome.status === "fulfilled" && !readOutcome.value.isError
            ? readOutcome.value.data
            : undefined;
        const released =
          refreshed !== undefined &&
          (refreshed.status === "ready" || refreshed.status === "partial") &&
          proposalOutcome.status === "fulfilled" &&
          quotesReadVersion(proposalOutcome.value.data, refreshed);
        setNotReadyBlocked(!released);
        if (released) {
          // The reader ran this check themselves and it released the block,
          // so the sentence naming that block has stopped being true —
          // leaving it up would be a false statement, not reassurance.
          setConfirmNotice(null);
        }
      },
    );
  }, [proposal.refetch, siteRead.refetch]);

  const confirm = useMutation({
    mutationFn: async (): Promise<CompanyProfile> => {
      const values = draftRef.current.values;
      const profileInput = {
        ...values,
        display_name: values.display_name.trim(),
        offer_summary: values.offer_summary.trim(),
        icp: values.icp.trim(),
        legal_name: values.legal_name.trim(),
        registered_address: values.registered_address.trim(),
        register_vat: values.register_vat.trim(),
        industry: values.industry.trim(),
      };
      const read = prevSnapshot.current;
      // When the proposal endpoint failed, the read snapshot carries the same
      // version pair, so the staged-confirm contract still holds.
      const proposalData =
        proposal.data ?? (read !== null ? proposalFromRead(read) : undefined);
      submittedProposalHashRef.current = proposalData?.proposal_hash;
      const result =
        read !== null &&
        (read.status === "ready" || read.status === "partial") &&
        proposalData?.draft_version !== undefined &&
        proposalData.proposal_hash !== undefined
          ? await api.POST("/company/site-reads/{readId}/confirm", {
              params: {
                path: { readId: read.id },
                header: { "Idempotency-Key": crypto.randomUUID() },
              },
              body: {
                draft_version: proposalData.draft_version,
                proposal_hash: proposalData.proposal_hash,
                profile: profileInput,
                selected_fact_keys: selectedFactKeys,
                resolutions: resolutionsFromAnswers(
                  read.comparisons,
                  clarify.answers,
                ),
              },
            })
          : await api.PUT("/company", { body: profileInput });
      const { data, error } = result;
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // Every fresh attempt starts clean: a notice from a PREVIOUS 409 must
    // not survive to describe THIS one, whether this click succeeds, fails
    // the same way again, or fails differently. Reachable only once a prior
    // block has actually lifted (Continue is disabled while one stands), so
    // resetting the blocks here is the belt on top of that suspender, not
    // the thing keeping a stale draft from resubmitting.
    onMutate: () => {
      setConfirmNotice(null);
      setSkewBlocked(false);
      setSkewStuck(false);
      setNotReadyBlocked(false);
    },
    onSuccess: finishConfirm,
    // A 409 always leaves the reader with something to do — never a dead
    // Continue button and a raw server string — and the server's own code
    // says which of its three refusals this is, so nothing here has to be
    // re-derived from a second request. `version_skew`: the draft the human
    // reviewed is stale, so the read is re-fetched right away
    // (`useCompanyRead`'s own poll already stopped once the read went
    // terminal, so nothing else will) and the NEXT press of Continue sends
    // whatever that fetch turns up. `already_confirmed`: an earlier
    // confirmation already created the company, so all that is left is to
    // load it. `not_confirmable`: this read has no draft to confirm, which
    // the identical submission cannot change — hence the block, and the
    // re-check that is the only thing able to lift it.
    onError: (error) => {
      const code = problemCodeOf(error);
      if (code === "version_skew") {
        // See refreshAfterSkew for the three outcomes this can settle into.
        setConfirmNotice("skew");
        setSkewBlocked(true);
        refreshAfterSkew();
        return;
      }
      if (code === "already_confirmed") {
        loadConfirmedCompany();
        return;
      }
      if (code === "not_confirmable") {
        setConfirmNotice("notReady");
        setNotReadyBlocked(true);
      }
    },
  });

  // Continue must not resubmit what the server has already refused in a way
  // an identical press cannot change: the proposal it rejected for version
  // skew (see refreshAfterSkew for the three outcomes that decide when
  // `skewBlocked` clears) or a read it says has no draft to confirm (see
  // recheckReadiness). Nor while the `already_confirmed` recovery is still
  // loading the company that confirmation created: the server has settled
  // this read, so a press there earns the same 409 and starts a second
  // lookup racing the first. A SETTLED "checkFailed" deliberately leaves
  // Continue armed: that load may simply have been unlucky, and pressing
  // again is a fair thing to do.
  const confirmBlocked = skewBlocked || notReadyBlocked || awaitingCompanyLoad;

  // The gate's own field is the ONE place a website address is typed — the
  // rail takes no free text, so there is no second entry point to keep in
  // step with this one. The gate hands back a bare host; canonicalising it
  // here keeps normalizeUrl the single spelling of "what a website address
  // is".
  const startFromGate = useCallback(
    (host: string) => {
      const norm = normalizeUrl(host);
      if (!norm.ok || startRead.isPending) {
        return;
      }
      setDraft((current) => changeDraftField(current, "website", norm.full));
      dispatch({ type: "URL_SUBMITTED", url: norm.full });
      startRead.mutate(norm.full);
    },
    [dispatch, setDraft, startRead],
  );

  const read = siteRead.data ?? startRead.data ?? null;
  legalEntitiesRef.current = read?.legal_entities ?? [];
  const missing = missingRequiredFields(draft.values);
  const readBroken = startRead.isError || siteRead.isError;

  const gateNotice = gateNoticeFor({
    state,
    read,
    startError: startRead.isError ? safeStartError(startRead.error, t) : null,
    translate: t,
    failedWithDetail: (detail) => t("ob.gate.startFailed", { detail }),
    pausedWithDetail: (detail) => t("ob.gate.readPaused", { detail }),
  });

  // The review renders even when the proposal endpoint failed: the site-read
  // snapshot carries the same evidence-gated mapping, just with no
  // server-detected open questions.
  const reviewProposal = useMemo(() => {
    if (proposal.data) {
      return proposal.data;
    }
    if (read && (read.status === "ready" || read.status === "partial")) {
      return proposalFromRead(read);
    }
    return null;
  }, [proposal.data, read]);

  // The rail's to-do list is the review board's own outstanding rows, read
  // through the exact same `rowFor`/`isWork` pair confirm-card.tsx's section
  // nav counts with — never a second idea of "still needs you" computed from
  // `missing` alone. `byName` mirrors confirm-card's own construction so a
  // field's state can never differ between the two surfaces.
  //
  // EVERY row is built once and the outstanding ones are a filter of that list,
  // rather than two walks of `reviewFields()`. The deck asks about the work and
  // the article states the whole record, and the two describing the same field
  // differently is exactly the drift this pair exists to prevent.
  const allRows = useMemo<readonly ReviewRow[]>(() => {
    if (reviewProposal === null) {
      return [];
    }
    const byName = new Map(
      evidencedFields(reviewProposal.fields)
        .filter((field) => isCompanyField(field.field, draft.values))
        .map((field) => [field.field, field]),
    );
    return reviewFields().map((field) => rowFor(field, draft, byName, t));
  }, [reviewProposal, draft, t]);
  const attentionRows = useMemo<readonly ReviewRow[]>(
    () => allRows.filter((row) => isWork(row.state)),
    [allRows],
  );
  // The ONLY rows that stop the human continuing: `confirmCompanySiteRead`
  // 422s exactly when one of REQUIRED_FIELDS is still empty — checked here
  // against `isRequired` itself, the same source the server enforces
  // against, never assumed from `row.state === "required"` alone. Every
  // other outstanding row (optional-empty, weak-confidence) is advisory:
  // worth a look, never an obstacle.
  const blocking = attentionRows.filter(blocksConfirm);
  const advisory = attentionRows.filter((row) => !blocksConfirm(row));
  // The gate's own third counter, and deliberately the SAME list the deck
  // renders one card at a time (`deckCards(blocking, advisory)`) rather than a
  // second tally computed from the read directly — the two counting outstanding
  // work differently is exactly the drift `deckCards` already exists to
  // prevent one level down. `attentionRows` (and so `blocking`/`advisory`) is
  // empty for most of a read simply because `reviewProposal` is still null —
  // that is "not known yet", not "nothing to ask about", and the gate must be
  // able to tell the two apart rather than showing a zero it has not earned.
  const uncertainCount =
    reviewProposal === null ? undefined : blocking.length + advisory.length;

  // The runtime bar keeps the live read total unless a clarify authorization
  // saw more calls — answering a decision is a real model round trip too,
  // and its own reply is a point-in-time copy, never a freeze.
  const readRuntime = read?.ai_runtime;
  const clarifyRuntime = clarify.runtime;
  const runtime =
    clarifyRuntime &&
    (!readRuntime || clarifyRuntime.call_attempts >= readRuntime.call_attempts)
      ? clarifyRuntime
      : readRuntime;

  // Entries already present at mount are transcript, not news. Freezing their
  // ids on the first render is what stops a leftover crawl line — which names
  // fields, and is very often the last thing said before the review appears —
  // from pulsing and scrolling the board the reader has only just arrived at.
  const mountedEntryIds = useRef<ReadonlySet<string> | null>(null);
  if (mountedEntryIds.current === null) {
    mountedEntryIds.current = new Set(state.thread.map((entry) => entry.id));
  }
  const preRendered = mountedEntryIds.current;
  const lastEntry = state.thread.at(-1);
  // A highlight belongs to the dossier: the review and decision scenes replace
  // that body outright, so one aimed at a scene no longer on screen would land
  // on whatever the new one happens to render.
  const dossierShowing =
    state.phase !== "co.review" && state.phase !== "co.clarify";
  const highlight = useMemo<FindingHighlight | null>(() => {
    if (
      dossierShowing &&
      lastEntry?.kind === "narration" &&
      !preRendered.has(lastEntry.id) &&
      lastEntry.findingIds !== undefined &&
      lastEntry.findingIds.length > 0
    ) {
      return { key: lastEntry.id, ids: lastEntry.findingIds };
    }
    return null;
  }, [lastEntry, dossierShowing, preRendered]);

  const pendingId = state.pendingQuestion?.id ?? null;
  // The review card's own "still open" list, recomputed here from the exact
  // same inputs (the proposal's open questions, the pending id, the recorded
  // answers) it derives its own count from — not a second tally the rail
  // could drift from once an answer lands.
  const openReviewQuestions = (reviewProposal?.open_questions ?? []).filter(
    (question) =>
      question.id !== pendingId &&
      !clarify.answers.some((answer) => answer.clarifyId === question.id),
  );
  /**
   * Whether a confirm would be refused, spelled ONCE because two surfaces ask.
   *
   * The deck and the whole-profile card are two ways into the same submission,
   * and they had two copies of this expression. The copies drifted immediately:
   * the deck's omitted `confirmBlocked`, so after a `version_skew` or
   * `not_confirmable` 409 its button came straight back to life and offered to
   * resubmit the draft the server had just refused — on the surface most
   * readers see, while the card behind it correctly stayed shut.
   *
   * Every term is a reason the SERVER would refuse:
   * - a required field is still empty, which `confirmCompanySiteRead` 422s on;
   * - a question the server still considers open is stranded, for the one
   *   render between a clarify settling and its successor landing;
   * - the act is not on a phase that has anything to confirm;
   * - the server has already refused in a way pressing again cannot change.
   */
  const confirmRefused =
    missing.length > 0 ||
    !(state.phase === "co.review" || state.phase === "co.manual") ||
    confirmBlocked;
  // What the rail names when Confirm is pressed early: the required fields
  // still empty, by label. Open questions are NOT here — the server saves a
  // record with a question unanswered, and a question the assistant could not
  // settle must not hold the reader on this screen for good; the rail names
  // them as left open instead. The server's own standing refusal is not here
  // either: it holds the button (`confirmHeld`) and explains itself in the
  // notice, with the retry that can actually change it.
  const confirmBlockers = blocking.map((row) => row.label);
  const confirmHeld =
    confirmBlocked ||
    !(state.phase === "co.review" || state.phase === "co.manual");

  const presence = presenceFor(state, { read, readBroken });

  // The gate and the read theatre are the company act's first face. It is
  // full-screen and deliberately has no thread, no panel and no composer:
  // before there is anything sourced to review, a two-column workbench would be
  // showing the reader an empty dossier and asking them to trust it.
  //
  // ONE return for both faces, because they are one column — the read replaces
  // the question below a Core and a headline that never move. Two returns would
  // put two component types at the same position and remount everything between
  // them; OnboardingGate's GateColumn documents what that costs.
  //
  // The condition is the whole span before there is anything to review, and
  // NOTHING else: an in-flight POST or an unarrived first snapshot are both just
  // "still waiting", so they keep the screen the reader is already on. Deriving
  // it from whether the manual escape is offered — which is suppressed while a
  // start is in flight — is what used to drop the reader onto an empty workbench
  // for the length of one request.
  const beforeReview =
    state.phase === "co.intro" || state.phase === "co.reading";
  // Only asked before there is anything to review. Once a read is running the
  // installation is plainly configured, and a query fired then would be a
  // request per render of a screen whose answer cannot have changed.
  const setup = useInstallationSetup();
  const platformDeclined = usePlatformDeclined();
  const setupOutstanding =
    beforeReview && outstandingStep(setup.data, platformDeclined) !== undefined;
  const scanning =
    state.phase === "co.reading" && state.activeReadId !== null && read
      ? { read, host: normalizeUrl(read.root_url).host, locale }
      : undefined;
  // A run the machine owns whose first snapshot has not arrived: the Core keeps
  // working and the question stays put rather than the column changing shape
  // twice in half a second.
  const awaitingRead =
    state.phase === "co.reading" && state.activeReadId !== null && !read;

  // BEFORE the website question, and before the read theatre: an installation
  // that has bound no model cannot perform a cold-start read at all, so asking
  // for a website first would take an answer and then refuse to act on it.
  //
  // The condition and the component read the SAME predicate, which is why this
  // can sit in front of the gate rather than beside it: `outstandingStep`
  // answers only with a step the component has a panel for, so there is no
  // report that keeps this branch taken while the component below draws
  // nothing.
  if (setupOutstanding) {
    return <InstallationSetup />;
  }

  if (beforeReview) {
    return (
      <OnboardingGate
        name={me.data?.user.display_name}
        running={startRead.isPending || awaitingRead}
        notice={gateNotice}
        configuredModel={configuredModel}
        scan={scanning}
        readBroken={readBroken}
        uncertainCount={uncertainCount}
        onSubmit={startFromGate}
        onManual={() => dispatch({ type: "MANUAL_CHOSEN" })}
        onRetryRead={() => siteRead.refetch()}
      />
    );
  }

  // ONE scene on the surface at a time — the prototype's rule. A pending
  // decision owns the whole surface; the review owns it after; the thread
  // beside them stays what a rail can carry: narration and history. Every
  // unresolved "question" entry is FILTERED from the rail, not only the one
  // matching the machine's current pendingQuestion — the reducer re-asks a
  // clarify by APPENDING a fresh entry rather than retiring the old one (a
  // background poll can re-issue the same clarify under a new id), so an
  // exact-id filter alone still lets a superseded, never-answered re-ask
  // render as a second, disabled copy of the same candidate list. A
  // resolved entry (answered or dismissed) is history and stays; the moment
  // one settles it returns to the transcript.
  const decision =
    state.phase === "co.clarify" && state.pendingQuestion !== null ? (
      <DecisionScene
        question={state.pendingQuestion}
        onAnswer={handleAnswer}
        onDismiss={handleDismiss}
        // The entity candidates carry their address, registry number and
        // imprint quote on the read; resolving the option's value back to its
        // candidate attaches each card's detail. Through the same matcher the
        // pick itself uses, so a card can never show details for a candidate
        // the pick would then fail to find — or show none for one it would.
        // Any other clarify has no candidate to attach and its cards render
        // as name-only.
        factsOf={(value) => {
          const entity = legalEntityForOption(
            read?.legal_entities ?? [],
            value,
          );
          return entity === undefined
            ? null
            : {
                meta: entity.registered_address,
                // The card has one slot for a number, so it shows the
                // registry identity and falls back to the tax one: an entity
                // whose notice printed only a VAT ID would otherwise be a
                // bare name at the moment somebody has to pick between two.
                mono: entity.register_number ?? entity.vat_number,
                snippet: entity.evidence_snippet,
                source: entity.source_url,
              };
        }}
      />
    ) : null;
  // The generic "I could not save that: {detail}" banner, but ONLY for a
  // confirm failure this driver has not already turned into something more
  // useful: `confirmNotice` covers version_skew (a fresh fetch is already in
  // flight; the pane's own refusal notice says so) and the other two
  // documented 409s, so this stays null for those rather than doubling the
  // message.
  // `already_confirmed` is the one refusal that carries no notice while it
  // resolves — its whole recovery is the company lookup — and that lookup
  // ends either in the review's exit or in the "checkFailed" notice, so a
  // failure banner over it would report a save that in fact went through.
  const confirmBannerMessage =
    confirm.isError && confirmNotice === null && !awaitingCompanyLoad
      ? problemMessageOf(confirm.error, t)
      : null;
  // The action each resolved-409 notice offers, or none. A "skew" notice
  // earns one only once its automatic refresh has already run and left the
  // block standing (`skewStuck`) — before that there is nothing left to ask
  // for, and Continue itself is the next step. The other two always do: for
  // "notReady" the re-check is the reader's ONLY route, because Continue is
  // blocked, and for "checkFailed" the load is the only thing that can turn
  // "I could not load it" into the company itself.
  const noticeRetries: Readonly<Record<ConfirmNotice, RefusalRetry | null>> = {
    skew: skewStuck
      ? { run: refreshAfterSkew, busy: awaitingProposalRefresh }
      : null,
    notReady: { run: recheckReadiness, busy: awaitingReadinessCheck },
    checkFailed: { run: loadConfirmedCompany, busy: awaitingCompanyLoad },
  };
  // A confirm 409 this driver already turned into a next step rather than a
  // bare failure — see the `confirm` mutation's onError, refreshAfterSkew,
  // recheckReadiness and loadConfirmedCompany for which server state each one
  // names, and why none of them is "fix your input and press the same button
  // again".
  //
  // It travels to the WORK SURFACE, not to the transcript beside it. The
  // control that earned the refusal is on the board, and the rail is a
  // scroller the reader pressing Continue is not looking at — a sentence
  // there explained the block to nobody, which is how two rejections
  // thirteen seconds apart read as a button that had simply stopped working.
  // One copy, on the pane: the same sentence in two places only makes the
  // reader decide which of them is about their press.
  const refusal: ConfirmRefusal | null =
    confirmNotice === null
      ? null
      : {
          message: t(confirmNoticeKey(confirmNotice, skewStuck)),
          retry: noticeRetries[confirmNotice],
        };
  // The confirm stop, as a deck by default and as the whole profile on ask.
  //
  // The deck is the front door because the read already knows which fields it
  // could not settle, and putting the other hundred on screen beside them asked
  // a reader to find six answers inside a wall. The wall is still HERE, one
  // press away: it is where a field is edited freely and a fact is unticked,
  // and the server wants both of those from somewhere.
  const cards = deckCards(blocking, advisory);
  // The same mapping over EVERY row, so the deck can still draw the card it is
  // standing on after that field stops being outstanding. Built from `allRows`
  // rather than kept as a copy: what the reader types has to reach the control
  // it was typed into.
  const cardOf = (field: CompanyFieldName) =>
    deckCards(
      allRows.filter((row) => row.field === field),
      [],
    )[0];
  // The mark at the head of the record, for both of its faces: the site the
  // read ran on, and the logo it resolved from there when it found one.
  // Undefined before a read exists, and the digest draws the monogram.
  const identity =
    read === null
      ? undefined
      : { rootUrl: read.root_url, logoUrl: read.logo_url };
  const reviewScene =
    state.phase === "co.review" && reviewProposal ? (
      artifactMode === "dossier" ? (
        <ReviewDeck
          cards={cards}
          cardOf={cardOf}
          settled={selectedFactKeys.length}
          onField={(field, value) =>
            setDraft((current) => changeDraftField(current, field, value))
          }
          onDone={() => confirm.mutate()}
          blockers={confirmBlockers}
          held={confirmHeld}
          openQuestions={openReviewQuestions.length}
          // EVERY row, not just the outstanding ones: the article is what the
          // record says, and a version of it that showed only the unanswered
          // half would be the deck's own list again in prose.
          digest={(active) => (
            <ProfileDigest
              rows={allRows}
              active={active}
              identity={identity}
              onReadWhole={() => {
                // A manual ask to read the whole record, not a settle — any
                // earlier "Settle it" target is stale the moment the reader
                // leaves the deck this way.
                setGoToField(null);
                setArtifactMode("profile");
              }}
            />
          )}
          pending={confirm.isPending}
          goTo={goToField ?? undefined}
        />
      ) : artifactMode === "profile" ? (
        // The whole record, read as the two-column document it is. A reader
        // who asks to see everything is READING, and the editing wall
        // answered that ask with a hundred controls: the two are different
        // acts and now have different doors.
        <div className="ob-scene">
          {/* The way back to the deck. The whole record is somewhere a reader
              ASKED to go, so it needs a door out that is not the browser's back
              button: without one the deck's tray, and the count of what is
              still open, are unreachable from here. */}
          <Button variant="ghost" onClick={() => setArtifactMode("dossier")}>
            {t("ob.deck.backToOpen")}
          </Button>
          {/* `read` rather than nothing: this is the door a reader asked for
              to see the WHOLE record, and the crawl found far more than the
              twenty-odd fields the deck ever asks about. The companion beside
              the deck never gets this prop, on purpose — it answers "what did
              my answer just land in", and the facts, people and legal
              entities the deck never touches would bury that under the whole
              archive. `onSettle` is the reverse door: pressing it on an
              unanswered line switches the deck back on AND points it at that
              field, through the same `goToField` the deck's own "Settle it"
              route uses. */}
          <ProfileDigest
            rows={allRows}
            read={read ?? undefined}
            identity={identity}
            onSettle={settleField}
            onField={(field, value) =>
              setDraft((current) => changeDraftField(current, field, value))
            }
          />
          {/* Which of the crawl's facts become company context is the one
              decision the article does not take; the board is where a fact is
              ticked, so that is what its door is called. */}
          <Button variant="ghost" onClick={() => setArtifactMode("record")}>
            {t("ob.digest.pickFacts")}
          </Button>
          {/* One Save for every line corrected in place, and only once one
              has been: the deck's own Confirm is the way onward for a reader
              who changed nothing, and a second button saying the same thing
              beside an untouched record would be a choice with no difference. */}
          {draft.edited.size > 0 && (
            <WayOnward
              label={t("ob.digest.saveChanges")}
              pendingLabel={t("ob.s1.saving")}
              pending={confirm.isPending}
              blockers={confirmBlockers}
              held={confirmHeld}
              stillNeeded={(fields) =>
                t("ob.deck.stillNeeded", { fields: fields.join(", ") })
              }
              note={
                <p className="ob-stage-hint">
                  {t("ob.digest.changed", {
                    count: formatNumber(draft.edited.size, locale),
                  })}
                </p>
              }
              onGo={() => confirm.mutate()}
            />
          )}
        </div>
      ) : (
        <div className="ob-scene">
          <Button variant="ghost" onClick={() => setArtifactMode("profile")}>
            {t("ob.deck.backToRecord")}
          </Button>
          <CompanyConfirmCard
            proposal={reviewProposal}
            draft={draft}
            answers={clarify.answers}
            read={read}
            selectedFactKeys={selectedFactKeys}
            setSelectedFactKeys={setSelectedFactKeys}
            missingRequired={missing}
            setField={(field, value) =>
              setDraft((current) => changeDraftField(current, field, value))
            }
            onAcceptAll={() => confirm.mutate()}
            pending={confirm.isPending}
            authorizing={clarify.authorizing || confirmBlocked}
            error={confirmBannerMessage}
          />
        </div>
      )
    ) : null;
  return (
    <ConversationWorkbench
      core={presence.core}
      progress={presence.progress}
      railState={state}
      status={
        readBroken
          ? t("ob.readStatus.failed")
          : read
            ? t(`ob.readStatus.${read.status}`)
            : t("ob.ai.ready")
      }
      runtime={runtime}
      {...boardHeading(state, t)}
    >
      {
        <CompanyActArtifact
          mode={artifactMode}
          manual={state.phase === "co.manual"}
          review={decision ?? reviewScene}
          read={read}
          draft={draft}
          setField={(field, value) =>
            setDraft((current) => changeDraftField(current, field, value))
          }
          onPickEntity={(entity) =>
            setDraft((current) => draftWithLegalEntity(current, entity))
          }
          selectedFactKeys={selectedFactKeys}
          setSelectedFactKeys={setSelectedFactKeys}
          missingRequired={missing}
          highlight={highlight}
          onSwitchMode={setArtifactMode}
          onConfirm={() => confirm.mutate()}
          confirmPending={confirm.isPending}
          confirmDisabled={confirmRefused}
          saveError={confirmBannerMessage}
          refusal={refusal}
        />
      }
      {/* The list of what still wants an answer is NOT here: the deck IS that
          list, met one card at a time and counted in its own tray. Printing it
          again underneath was the same outstanding work said twice, in a flat
          order the reader was not being walked through. A failure that needs a
          retry has no such home, so those stay. */}
      {startRead.isError && (
        <p className="ob-conv-notice" role="alert">
          {t("ob.gate.startFailed", {
            detail: safeStartError(startRead.error, t),
          })}
        </p>
      )}
      {clarify.failure && (
        <p className="ob-conv-notice" role="alert">
          {clarify.failure.kind === "request"
            ? t("ob.conv.clarify.applyFailed", {
                detail: clarify.failure.detail,
              })
            : t("ob.conv.clarify.applyMissing")}
        </p>
      )}
    </ConversationWorkbench>
  );
}

/**
 * What the room says this screen is, per phase.
 *
 * THE QUESTION IS THE TITLE while one is pending. A clarify is the whole reason
 * the screen exists, and the alternative — a standing heading with the question
 * repeated inside a card below it — is the same sentence twice with the cards
 * pushed off the fold. `DecisionScene` renders no heading of its own and labels
 * its options by pointing at this one.
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

// The one predicate for "does this row stop the human continuing" — the
// server's `confirmCompanySiteRead` 422s exactly when one of REQUIRED_FIELDS
// (`isRequired`) is still empty, so that is the whole test. Never widened to
// `row.state === "required"` on its own: `rowFor` happens to assign that
// state for exactly this case today, but the check goes through `isRequired`
// so a future change to that naming cannot silently stop matching what the
// server actually enforces.
function blocksConfirm(row: ReviewRow): boolean {
  return isRequired(row.field) && row.value.trim() === "";
}
