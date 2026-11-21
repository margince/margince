// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type {
  ConversationAct,
  ConversationPhase,
  ConversationState,
  ResumePoint,
} from "./conversation-types";

// Which act owns each landing point; the act is derived from the phase so the
// pair can never disagree.
const landingActs: Record<ResumePoint, ConversationAct> = {
  "bs.ask": "basis",
  "in.ask": "invite",
  "tm.ask": "team",
  "vo.collecting": "voice",
  "cn.consent": "connect",
};

// The landing points that exist only on the creator's route. Every event in
// them is illegal for a member, who lands on their first own act instead.
const creatorOnlyLandings = new Set<ResumePoint>([
  "bs.ask",
  "in.ask",
  "tm.ask",
]);

/** Where a journey lands for a point it asks for, restored or live. Without
 *  one, a creator opens basis and a member voice; a journey with no voice act
 *  goes on to connect wherever voice would open. */
export function landingFor(
  state: ConversationState,
  target: ResumePoint | undefined,
): { act: ConversationAct; phase: ConversationPhase } {
  let phase = target ?? (state.memberPath ? "vo.collecting" : "bs.ask");
  if (state.memberPath && creatorOnlyLandings.has(phase)) {
    phase = "vo.collecting";
  }
  if (!state.voiceStep && landingActs[phase] === "voice") {
    phase = "cn.consent";
  }
  return { act: landingActs[phase], phase };
}
