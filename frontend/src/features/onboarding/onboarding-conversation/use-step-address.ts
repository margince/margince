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
    if (prev.act === "connect" && state.act === "voice") {
      cameBack.current = true;
    }
    const arrived = routeId !== prev.routeId;
    const echo =
      arrived && routeId !== undefined && written.current.includes(routeId);
    if (echo) {
      written.current = written.current.slice(
        written.current.indexOf(routeId) + 1,
      );
    }
    const address = STEP_ADDRESSES[state.act];
    if (address === undefined || routeId === address) {
      return;
    }
    const moved = arrived && !echo;
    if (
      moved &&
      routeId === STEP_ADDRESSES.voice &&
      isLegal(state, { type: "CONNECT_BACK" })
    ) {
      dispatch({ type: "CONNECT_BACK" });
      return;
    }
    const forward =
      moved && cameBack.current && routeId === STEP_ADDRESSES.connect
        ? forwardFromVoice(state)
        : null;
    if (forward !== null) {
      dispatch(forward);
      return;
    }
    const next = { screen: "onboarding", id: address } as const;
    written.current = [...written.current, address];
    // Only a step forward is a history entry, so Back has one to return to;
    // any other disagreement is corrected in place.
    if (prev.act === "voice" && state.act === "connect") {
      navigate(next);
    } else {
      navigateReplacing(next);
    }
  }, [state, dispatch, routeId, navigateReplacing]);
}
