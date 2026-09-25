// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCompanyContextCapabilities } from "./company-context";
import { useOnboardingProgress } from "./onboarding";

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
): Readonly<{
  pending: boolean;
  unfinished: boolean;
}> {
  const rollout = useCompanyContextCapabilities(authed);
  const walkable = wanted && rollout.data?.onboarding_enabled === true;
  const progress = useOnboardingProgress(walkable);
  return {
    pending: wanted && (rollout.isPending || (walkable && progress.isPending)),
    unfinished:
      walkable &&
      progress.isSuccess &&
      (progress.data === null || progress.data.step !== "complete"),
  };
}
