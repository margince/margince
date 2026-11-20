// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useCompanyContextCapabilities } from "../../lib/companycontextreach";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  watchStored,
  writeStored,
} from "../../lib/storage";
import { useOnboardingProgress } from "./onboarding-progress";

const finishedKey = (userId: string) => ({
  family: STORAGE_KEYS.onboardingComplete,
  member: userId,
});

function useRememberedFinished(userId: string | undefined): boolean {
  const watch = useCallback(
    (changed: () => void) =>
      userId === undefined
        ? () => undefined
        : watchStored(finishedKey(userId), changed),
    [userId],
  );
  return useSyncExternalStore(
    watch,
    () => userId !== undefined && readStored(finishedKey(userId)) === "1",
  );
}

// The second half of the onboarding gate, for a DESCRIBED installation: a
// human whose own journey — voice, mailbox, preferences — is not recorded as
// finished is walked through it, a member invited later exactly as the
// creator was. `wanted` is false for a read seat: it cannot write the
// checkpoint the journey ends on, and a gate with no exit is a trap. It is
// false for an undescribed installation too, which the first half already
// gates. Below the `onboarding` rollout stage there is no journey to walk, only
// the manual company form, so the gate holds nobody there either.
//
// The rollout is asked once the session is `authed`, beside the company read
// rather than behind it; `pending` still counts it only where the gate needs it.
// A read that FAILED — the rollout's or the row's — does not gate: the shell
// renders, and the journey is asked for again on the next load. An unfinished
// row, or none, does.
export function useJourneyProgress(
  authed: boolean,
  wanted: boolean,
  userId: string | undefined,
): Readonly<{
  pending: boolean;
  unfinished: boolean;
}> {
  const rollout = useCompanyContextCapabilities(authed);
  const walkable = wanted && rollout.data?.onboarding_enabled === true;
  const progress = useOnboardingProgress(walkable);
  // Seen finished here: no wait, but an unfinished answer still gates and forgets.
  const remembered = useRememberedFinished(userId);
  const finished = progress.isSuccess && progress.data?.step === "complete";
  const unfinished = walkable && progress.isSuccess && !finished;
  // An answer held when a new user signs in is the last user's, signed-out gap or not.
  const [owner, setOwner] = useState({ userId, answeredAfter: 0 });
  if (userId !== undefined && owner.userId !== userId) {
    setOwner({
      userId,
      answeredAfter: owner.userId === undefined ? 0 : progress.dataUpdatedAt,
    });
  }
  const theirs = progress.dataUpdatedAt > owner.answeredAfter;
  useEffect(() => {
    if (userId === undefined || !theirs) return;
    if (finished) writeStored(finishedKey(userId), "1");
    else if (unfinished) removeStored(finishedKey(userId));
  }, [userId, theirs, finished, unfinished]);
  return {
    pending:
      wanted &&
      !remembered &&
      (rollout.isPending || (walkable && progress.isPending)),
    unfinished,
  };
}
