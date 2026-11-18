// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useCanWrite } from "../../lib/capability";
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

// Holds a reader of a DESCRIBED installation whose journey has a step left to
// take. Below the rollout stage, or after a FAILED read, nobody is held.
export function useJourneyProgress(
  authed: boolean,
  described: boolean | undefined,
  userId: string | undefined,
): Readonly<{
  pending: boolean;
  unfinished: boolean;
}> {
  // useCanWrite folds the seat in, and both read false until /me answers.
  const trainsVoice = useCanWrite("voice_profile", "create");
  const connectsMail = useCanWrite("activity", "create");
  const wanted = authed && described === true && (trainsVoice || connectsMail);
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
