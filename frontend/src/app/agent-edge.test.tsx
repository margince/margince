// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentEdge } from "./agent-edge";
import {
  AGENT_EDGE_STILL,
  clearAgentEdge,
  currentAgentEdge,
  publishAgentEdge,
} from "./agent-edge-signal";

// The margins are decoration with a job: they are the only place on a screen
// that says the agent is working, and the only place that says something has
// stopped for a person. What these cases hold is the difference between those
// two and silence, because silence is the default and a mark drawn by mistake
// would be the surface claiming work nobody asked for.

const edge = (container: HTMLElement) => container.querySelector(".agentedge");

beforeEach(() => {
  // Module state, so it survives between cases: a test that inherited the last
  // one's reading would pass for the wrong reason.
  clearAgentEdge();
});

afterEach(cleanup);

describe("the agent edge signal", () => {
  it("starts still, which is what a screen with no agent on it should say", () => {
    expect(currentAgentEdge()).toEqual(AGENT_EDGE_STILL);
  });

  it("keeps one object per reading, so a subscriber can compare by identity", () => {
    const first = currentAgentEdge();
    publishAgentEdge({ reading: false, register: "agent" });

    expect(currentAgentEdge()).toBe(first);
  });

  it("carries the reading it was published with", () => {
    publishAgentEdge({ reading: true, register: "agent" });

    expect(currentAgentEdge()).toEqual({ reading: true, register: "agent" });
  });

  it("carries the import's register, and drops it the moment the light goes out", () => {
    // A register means nothing while dark, so rest has ONE spelling whatever
    // the light was doing: a subscriber comparing by identity sees the still
    // object, and a test asserting rest need not know what ran before it.
    publishAgentEdge({ reading: true, register: "capture" });
    expect(currentAgentEdge()).toEqual({ reading: true, register: "capture" });

    publishAgentEdge({ reading: false, register: "capture" });
    expect(currentAgentEdge()).toBe(AGENT_EDGE_STILL);
  });

  it("publishes a change of register as a change, so a run starting mid-import reaches the margins", () => {
    publishAgentEdge({ reading: true, register: "capture" });
    const thin = currentAgentEdge();
    publishAgentEdge({ reading: true, register: "agent" });

    expect(currentAgentEdge()).not.toBe(thin);
    expect(currentAgentEdge().register).toBe("agent");
  });

  it("goes still when cleared, so a reading cannot outlive its session", () => {
    publishAgentEdge({ reading: true, register: "agent" });
    clearAgentEdge();

    expect(currentAgentEdge()).toEqual(AGENT_EDGE_STILL);
  });
});

describe("the agent edge", () => {
  it("draws nothing at all while nothing is happening", () => {
    const { container } = render(<AgentEdge />);

    expect(edge(container)).not.toBeNull();
    expect(container.querySelector("canvas")).toBeNull();
    // Nothing but the canvas: an unanswered queue used to close the margin into
    // a contour that stood for as long as the queue did, which on any real
    // installation is a permanent ring around the window. The rail says it in
    // words, with its count.
    expect(edge(container)?.children).toHaveLength(0);
  });

  it("lights on reading, and goes dark again when the work stops", () => {
    // The lit edge mounting IS the reading, so there is no attribute to check:
    // an element that only exists while work is in flight cannot fall out of
    // step with the fact it reports.
    const { container } = render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));
    expect(container.querySelector("canvas")).not.toBeNull();

    act(() => publishAgentEdge({ reading: false, register: "agent" }));
    expect(container.querySelector("canvas")).toBeNull();
  });

  it("draws the lit edge only while the agent is reading", () => {
    // A full-window fragment shader: cheap per frame, not free to have. A dark
    // edge has nothing to say, so an idle screen must not be paying for it.
    const { container } = render(<AgentEdge />);
    expect(container.querySelector("canvas")).toBeNull();

    act(() => publishAgentEdge({ reading: true, register: "agent" }));
    expect(container.querySelector("canvas")).not.toBeNull();

    act(() => publishAgentEdge({ reading: false, register: "agent" }));
    expect(container.querySelector("canvas")).toBeNull();
  });

  it("wears the static rim where the shader cannot run", () => {
    // jsdom answers `getContext("webgl2")` with null, which is the same answer a
    // locked-down browser and a refused compile give. The edge still has to say
    // the agent is working, so the fallback is a plain lit rim rather than
    // nothing: a decoration that vanishes takes a reading with it.
    const { container } = render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    expect(container.querySelector(".agentedge-still")).not.toBeNull();
  });

  it("tells the rim which register it is in, so the fallback can wear it too", () => {
    // The shader's frames never reach the DOM, so this attribute is the one
    // place the register is legible outside the loop: the static rim draws
    // its thinner spelling from it, and this test reads it the same way.
    const { container } = render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "capture" }));
    expect(
      container.querySelector("canvas")?.getAttribute("data-register"),
    ).toBe("capture");

    // A run starting mid-import is the agent's own work, and the rim says so.
    act(() => publishAgentEdge({ reading: true, register: "agent" }));
    expect(
      container.querySelector("canvas")?.getAttribute("data-register"),
    ).toBe("agent");
  });

  it("takes no pointer and is hidden from a screen reader", () => {
    // Everything it says is also said in words in the rail, so this is
    // decoration: a reader who cannot see it loses nothing, and one who can
    // must never have it swallow a click meant for the page underneath.
    const { container } = render(<AgentEdge />);

    expect(edge(container)?.getAttribute("aria-hidden")).toBe("true");
  });

  it("follows the signal from more than one place on the page at once", () => {
    // Two mounted margins would be a bug, but the store is the thing under test
    // here: it has to serve every subscriber, not the last one to arrive.
    const first = render(<AgentEdge />);
    const second = render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    expect(first.container.querySelector("canvas")).not.toBeNull();
    expect(second.container.querySelector("canvas")).not.toBeNull();
  });

  it("stops hearing the signal once it is gone", () => {
    // A listener left in the set after unmount is a leak that React reports as
    // a state update on an unmounted component, and this store outlives every
    // component that reads it.
    const warn = vi.spyOn(console, "error").mockImplementation(() => {});
    const { unmount } = render(<AgentEdge />);
    unmount();
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
