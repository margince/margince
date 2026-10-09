import type { UseQueryResult } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { problemCodeOf } from "../common";
import type { RefusalRetry } from "./artifact";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type CompanyProfile = components["schemas"]["CompanyProfile"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];

/**
 * Which refusal a confirm 409 named, each with its own server code
 * (crm.yaml, confirmCompanySiteRead).
 *
 * - "skew" is `version_skew`: the draft changed under the human.
 * - "notReady" is `not_confirmable`: this read has no draft to confirm.
 * - "checkFailed" is what an `already_confirmed` leaves when the company that
 *   confirmation created could not be loaded.
 *
 * None is "fix your input and try again", so none takes the generic banner.
 */
export type ConfirmNotice = "skew" | "notReady" | "checkFailed";

export type RecoveredRefusal = Readonly<{
  messageKey: MessageKey;
  retry: RefusalRetry | null;
}>;

/**
 * Whether the version pair the next confirm would quote is this read's own,
 * the whole question a readiness re-check asks of the proposal half.
 *
 * A proposal endpoint that never answered needs no comparison. The confirm
 * then quotes the refreshed read's own pair, and no snapshot exists to go
 * stale.
 */
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

// A "skew" has two sentences, because the reader's next step changes once its
// refresh has run and the block is still there.
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

function recoveredRefusal(
  notice: ConfirmNotice,
  skewStuck: boolean,
  retry: RefusalRetry | null,
): RecoveredRefusal {
  return { messageKey: confirmNoticeKey(notice, skewStuck), retry };
}

type ConfirmRecoveryArgs = Readonly<{
  refetchRead: UseQueryResult<CompanySiteRead, Error>["refetch"];
  refetchProposal: UseQueryResult<Proposal, Error>["refetch"];
  finishConfirm: (profile: CompanyProfile) => void;
}>;

/**
 * Turns each confirm 409 into a next step, and holds Continue while pressing
 * it again would earn the same refusal.
 *
 * Each look below has one function, so the automatic run after a 409 and the
 * reader's own retry cannot drift apart. A notice clears only at the next
 * attempt or at the reader's own check that ends its state.
 */
export function useConfirmRecovery({
  refetchRead,
  refetchProposal,
  finishConfirm,
}: ConfirmRecoveryArgs) {
  const [notice, setNotice] = useState<ConfirmNotice | null>(null);
  // Whether a "skew" blocks the next press, and whether its refresh already
  // ran and left the block standing.
  const [skewBlocked, setSkewBlocked] = useState(false);
  const [skewStuck, setSkewStuck] = useState(false);
  // A `not_confirmable` always blocks: only a re-check that finds the read
  // confirmable again lifts it.
  const [notReadyBlocked, setNotReadyBlocked] = useState(false);
  // One flag per look in flight, so its retry cannot start a second one.
  const [awaitingProposalRefresh, setAwaitingProposalRefresh] = useState(false);
  const [awaitingReadinessCheck, setAwaitingReadinessCheck] = useState(false);
  const [awaitingCompanyLoad, setAwaitingCompanyLoad] = useState(false);
  // The hash this attempt submitted, so the skew refresh can tell a new draft
  // from the one the server rejected.
  const submittedProposalHashRef = useRef<string | undefined>(undefined);

  // The proposal is cached apart from the read, so both are refetched. Only a
  // different hash lifts the block. An unchanged hash or a failed refetch
  // leaves nothing new to resubmit, so `skewStuck` offers another look.
  const refreshAfterSkew = useCallback(() => {
    setAwaitingProposalRefresh(true);
    const submittedHash = submittedProposalHashRef.current;
    void Promise.allSettled([refetchRead(), refetchProposal()]).then(
      ([, proposalOutcome]) => {
        setAwaitingProposalRefresh(false);
        const refreshedHash =
          proposalOutcome.status === "fulfilled"
            ? proposalOutcome.value.data?.proposal_hash
            : undefined;
        const landedNewDraft =
          refreshedHash !== undefined && refreshedHash !== submittedHash;
        setSkewBlocked(!landedNewDraft);
        setSkewStuck(!landedNewDraft);
      },
    );
  }, [refetchRead, refetchProposal]);

  // The server already named this read confirmed, so loading the company it
  // created is the whole recovery. A failed load says so instead of leaving
  // the reader on a Continue that earns the same 409.
  const loadConfirmedCompany = useCallback(() => {
    setAwaitingCompanyLoad(true);
    const load = async () => {
      const { data } = await api.GET("/company");
      if (data === undefined) {
        setNotice("checkFailed");
        return;
      }
      finishConfirm(data);
    };
    void load()
      .catch((loadError) => {
        // The cause belongs in the console, never in the reader's sentence.
        console.error("confirmed-company load failed", loadError);
        setNotice("checkFailed");
      })
      .finally(() => setAwaitingCompanyLoad(false));
  }, [finishConfirm]);

  // The only thing that lifts a `not_confirmable` block. The confirm sends a
  // confirmable read and a version pair, so both are checked. A failed refetch
  // proves nothing, because react-query keeps the snapshot the server refused.
  const recheckReadiness = useCallback(() => {
    setAwaitingReadinessCheck(true);
    void Promise.allSettled([refetchRead(), refetchProposal()]).then(
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
          // The block this notice names has ended, so the sentence is no
          // longer true.
          setNotice(null);
        }
      },
    );
  }, [refetchProposal, refetchRead]);

  // A fresh attempt starts clean, so a notice from an earlier 409 cannot
  // describe this one.
  const resetBeforeAttempt = () => {
    setNotice(null);
    setSkewBlocked(false);
    setSkewStuck(false);
    setNotReadyBlocked(false);
  };

  const recordSubmitted = (proposalHash: string | undefined) => {
    submittedProposalHashRef.current = proposalHash;
  };

  // The server's own code says which refusal this is, so nothing is
  // re-derived from a second request. Any other failure takes the banner.
  const handleRefusal = (error: Error) => {
    const code = problemCodeOf(error);
    if (code === "version_skew") {
      setNotice("skew");
      setSkewBlocked(true);
      refreshAfterSkew();
      return;
    }
    if (code === "already_confirmed") {
      loadConfirmedCompany();
      return;
    }
    if (code === "not_confirmable") {
      setNotice("notReady");
      setNotReadyBlocked(true);
    }
  };

  // A "skew" offers a retry only once its refresh has left the block standing.
  // The other two always do: each is the reader's only route forward.
  const retries: Readonly<Record<ConfirmNotice, RefusalRetry | null>> = {
    skew: skewStuck
      ? { run: refreshAfterSkew, busy: awaitingProposalRefresh }
      : null,
    notReady: { run: recheckReadiness, busy: awaitingReadinessCheck },
    checkFailed: { run: loadConfirmedCompany, busy: awaitingCompanyLoad },
  };
  return {
    notice,
    refusal:
      notice === null
        ? null
        : recoveredRefusal(notice, skewStuck, retries[notice]),
    // Continue must not resubmit what the server refused in a way an identical
    // press cannot change, nor race the company load. A settled "checkFailed"
    // leaves it armed, since that load may simply have been unlucky.
    blocked: skewBlocked || notReadyBlocked || awaitingCompanyLoad,
    awaitingCompanyLoad,
    resetBeforeAttempt,
    recordSubmitted,
    handleRefusal,
  };
}
