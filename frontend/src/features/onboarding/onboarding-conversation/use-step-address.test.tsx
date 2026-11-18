/** @vitest-environment happy-dom */
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { useReducer } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useRoute } from "../../../lib/router";
import {
  type ConversationEvent,
  conversationReducer,
  initialConversationState,
} from "./conversation-machine";
import { run } from "./test-fixtures";
import { useStepAddress } from "./use-step-address";

// The real reducer and the real router under the hook. What is proven: the
// address and the act on screen agree, whichever of the two moved.

const onVoice = run([
  { type: "START", memberPath: true, companyConfirmed: true },
  { type: "RESUME", target: "vo.collecting" },
]);

let send: (event: ConversationEvent) => void = () => {};

function Harness() {
  const [state, dispatch] = useReducer(conversationReducer, onVoice);
  const route = useRoute();
  useStepAddress(state, dispatch, route.id);
  send = dispatch;
  return <p data-testid="act">{state.act}</p>;
}

const actShown = () => screen.getByTestId("act").textContent;

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/onboarding/company");
});
afterEach(() => {
  cleanup();
});

describe("each step at its own address", () => {
  it("names the step on screen, whatever address it opened on", async () => {
    render(<Harness />);
    await waitFor(() =>
      expect(globalThis.location.hash).toBe("#/onboarding/voice"),
    );
    const entries = globalThis.history.length;
    act(() => send({ type: "VOICE_SKIPPED" }));
    await waitFor(() =>
      expect(globalThis.location.hash).toBe("#/onboarding/connect"),
    );
    expect(actShown()).toBe("connect");
    // A step forward is an entry of its own, which the browser's Back returns from.
    expect(globalThis.history.length).toBe(entries + 1);
  });

  it("takes the browser's move back to the voice address as a step back", async () => {
    render(<Harness />);
    act(() => send({ type: "VOICE_SKIPPED" }));
    await waitFor(() =>
      expect(globalThis.location.hash).toBe("#/onboarding/connect"),
    );

    act(() => {
      globalThis.location.hash = "#/onboarding/voice";
    });
    await waitFor(() => expect(actShown()).toBe("voice"));
    expect(globalThis.location.hash).toBe("#/onboarding/voice");
  });

  it("follows the Back button's own step back", async () => {
    render(<Harness />);
    act(() => send({ type: "VOICE_SKIPPED" }));
    act(() => send({ type: "CONNECT_BACK" }));
    await waitFor(() =>
      expect(globalThis.location.hash).toBe("#/onboarding/voice"),
    );
    expect(actShown()).toBe("voice");
  });

  it("puts a typed step address back to the step the journey stands on", async () => {
    render(<Harness />);
    act(() => {
      globalThis.location.hash = "#/onboarding/connect";
    });
    await waitFor(() =>
      expect(globalThis.location.hash).toBe("#/onboarding/voice"),
    );
    expect(actShown()).toBe("voice");
  });
});

it("leaves an act with no step address where it was reached", () => {
  function Welcome() {
    const [state, dispatch] = useReducer(
      conversationReducer,
      initialConversationState,
    );
    useStepAddress(state, dispatch, useRoute().id);
    return null;
  }
  render(<Welcome />);
  expect(globalThis.location.hash).toBe("#/onboarding/company");
});
