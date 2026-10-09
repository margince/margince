import { useCallback, useMemo, useRef, useState } from "react";
import type { components } from "../../api/schema";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import type { ArtifactMode, FindingHighlight } from "./artifact";
import type { ConversationState } from "./conversation-machine";
import type { WizardPersistInput } from "./use-wizard-state";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type ProposalJoin = "pending" | "ready" | "failed";

/**
 * Whether the wizard-state write joining the running read has landed.
 *
 * The proposal endpoint joins through persisted wizard state, so a read is
 * recorded the moment it starts. The proposal fetch waits for that write,
 * because a stale join would serve the previous read.
 */
export function useProposalJoin(
  state: ConversationState,
  persist: (input: WizardPersistInput) => Promise<boolean>,
  draftRef: Readonly<{ current: CompanyDraft }>,
) {
  // A run the machine owns at mount was persisted when it started, which is
  // how restore found it, so its join is already in place.
  const [proposalJoin, setProposalJoin] = useState<ProposalJoin>(() =>
    state.activeReadId !== null ? "ready" : "pending",
  );
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
    [persist, draftRef],
  );
  return { proposalJoin, onReadStarted };
}

/**
 * Which face of the review the artifact shows, and which field the deck opens
 * on next.
 *
 * The whole-record document's "Settle it" pill sets the settle target. Leaving
 * the deck any other way clears it, so a stale target cannot reach into a deck
 * the reader has since moved past.
 */
export function useArtifactNavigation() {
  const [artifactMode, setArtifactMode] = useState<ArtifactMode>("dossier");
  const [goToField, setGoToField] = useState<CompanyFieldName | null>(null);
  const settleField = useCallback((field: CompanyFieldName) => {
    setGoToField(field);
    setArtifactMode("dossier");
  }, []);
  const readWhole = () => {
    setGoToField(null);
    setArtifactMode("profile");
  };
  return { artifactMode, setArtifactMode, goToField, settleField, readWhole };
}

/**
 * The dossier finding the newest narration points at, if it should pulse.
 *
 * Entries present at mount are transcript, not news: freezing their ids keeps
 * a leftover crawl line from pulsing the board the reader has just reached.
 */
export function useFindingHighlight(
  state: ConversationState,
): FindingHighlight | null {
  const mountedEntryIds = useRef<ReadonlySet<string> | null>(null);
  if (mountedEntryIds.current === null) {
    mountedEntryIds.current = new Set(state.thread.map((entry) => entry.id));
  }
  const preRendered = mountedEntryIds.current;
  const lastEntry = state.thread.at(-1);
  // The review and decision scenes replace the dossier, so a highlight there
  // would land on whatever the new scene renders.
  const dossierShowing =
    state.phase !== "co.review" && state.phase !== "co.clarify";
  return useMemo<FindingHighlight | null>(() => {
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
}
