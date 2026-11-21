import type { Dispatch } from "react";
import { useEffect, useRef } from "react";
import { navigate } from "../../../lib/router";
import { useAddressWriters } from "../../../lib/urlstate";
import { isLegal } from "./conversation-legality";
import type {
  ConversationAct,
  ConversationEvent,
  ConversationState,
} from "./conversation-types";

// Each step has its own address, so the browser's Back returns from connect
// to voice. Every other act keeps the address it was reached by.
const STEP_ADDRESSES: Partial<Record<ConversationAct, string>> = {
  voice: "voice",
  connect: "connect",
};

// The event that walks on from the voice step, where one is legal right now.
function forwardFromVoice(state: ConversationState): ConversationEvent | null {
  const ways: readonly ConversationEvent[] = [
    { type: "VOICE_DONE" },
    { type: "VOICE_SKIPPED" },
  ];
  return ways.find((event) => isLegal(state, event)) ?? null;
}

// What the reader's own move to an address asks of the act, or null.
function arrivalEvent(
  state: ConversationState,
  routeId: string | undefined,
  cameBack: boolean,
): ConversationEvent | null {
  if (
    routeId === STEP_ADDRESSES.voice &&
    isLegal(state, { type: "CONNECT_BACK" })
  ) {
    return { type: "CONNECT_BACK" };
  }
  return routeId === STEP_ADDRESSES.connect && cameBack
    ? forwardFromVoice(state)
    : null;
}

// What the route's arrival at `to` asks: the writes still owed, and the act's
// move when the reader, not this hook, wrote the address.
function readArrival(
  written: string[],
  from: string | undefined,
  to: string | undefined,
  state: ConversationState,
  cameBack: boolean,
): { owed: string[]; event: ConversationEvent | null } {
  if (to === undefined || to === from) {
    return { owed: written, event: null };
  }
  const at = written.indexOf(to);
  return at === -1
    ? { owed: written, event: arrivalEvent(state, to, cameBack) }
    : { owed: written.slice(at + 1), event: null };
}

/** Keeps `#/onboarding/<step>` in step with the act. A disagreeing address is
 *  rewritten, except the browser's move back to voice, which moves the act. */
export function useStepAddress(
  state: ConversationState,
  dispatch: Dispatch<ConversationEvent>,
  routeId: string | undefined,
) {
  const seen = useRef({ act: state.act, routeId });
  // The addresses this hook wrote that the route has not reached yet, in order.
  const written = useRef<string[]>([]);
  // Forward to connect returns where the reader stood; a typed address does not.
  const cameBack = useRef(false);
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    const prev = seen.current;
    seen.current = { act: state.act, routeId };
    cameBack.current ||= prev.act === "connect" && state.act === "voice";
    const arrival = readArrival(
      written.current,
      prev.routeId,
      routeId,
      state,
      cameBack.current,
    );
    written.current = arrival.owed;
    const address = STEP_ADDRESSES[state.act];
    if (address === undefined || routeId === address) {
      return;
    }
    if (arrival.event !== null) {
      dispatch(arrival.event);
      return;
    }
    written.current = [...written.current, address];
    // Only a step forward is a history entry, so Back has one to return to;
    // any other disagreement is corrected in place.
    const forward = prev.act === "voice" && state.act === "connect";
    (forward ? navigate : navigateReplacing)({
      screen: "onboarding",
      id: address,
    });
  }, [state, dispatch, routeId, navigateReplacing]);
}
