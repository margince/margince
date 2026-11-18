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

/** Keeps `#/onboarding/<step>` in step with the act. A disagreeing address is
 *  rewritten, except the browser's move back to voice, which moves the act. */
export function useStepAddress(
  state: ConversationState,
  dispatch: Dispatch<ConversationEvent>,
  routeId: string | undefined,
) {
  const seen = useRef({ act: state.act, routeId });
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    const prev = seen.current;
    seen.current = { act: state.act, routeId };
    const address = STEP_ADDRESSES[state.act];
    if (address === undefined || routeId === address) {
      return;
    }
    if (
      routeId !== prev.routeId &&
      routeId === STEP_ADDRESSES.voice &&
      isLegal(state, { type: "CONNECT_BACK" })
    ) {
      dispatch({ type: "CONNECT_BACK" });
      return;
    }
    const next = { screen: "onboarding", id: address } as const;
    // Only a step forward is a history entry, so Back has one to return to;
    // any other disagreement is corrected in place.
    if (prev.act === "voice" && state.act === "connect") {
      navigate(next);
    } else {
      navigateReplacing(next);
    }
  }, [state, dispatch, routeId, navigateReplacing]);
}
