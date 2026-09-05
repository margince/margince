/** @vitest-environment jsdom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { AgentEdge } from "./agent-edge";
import {
  EDGE_LIGHT_KEY,
  edgeLightShown,
  setEdgeLightShown,
} from "./agent-edge-preference";
import { clearAgentEdge, publishAgentEdge } from "./agent-edge-signal";
import { AgentRail } from "./agentrail";
import { LABELS } from "./agentrail-copy";
import { meFixture } from "./mefixture";

// The margins are the one thing on a workspace screen that moves without being
// asked for, and they move around the whole window. Some people cannot work
// beside that and some simply do not want to, so the panel carries a switch —
// and what these cases hold is that the switch actually reaches the surface:
// off means nothing is drawn and nothing is paid for, on means the light comes
// back, and the answer outlives the panel that set it.

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

/** Every read the rail makes, answered healthy: this file is about the switch,
 *  not about a posture. */
function stubApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: { license: ["read"] } }));
      }
      if (pathname.endsWith("/assistant/profile")) {
        return jsonResponse({
          name: "Margince",
          kind: "ai",
          state: "configured",
          inference_mode: "cloud",
          providers: [],
        });
      }
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({ running: [], recent: [] });
      }
      if (pathname.endsWith("/installation/license")) {
        return jsonResponse({
          state: "valid",
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        });
      }
      if (pathname.endsWith("/ai/usage")) {
        return jsonResponse({
          days: [],
          budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
        });
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
}

/** The rail and the margins on one page, which is how the shell mounts them:
 *  the switch is in the panel, and the surface it governs is elsewhere. */
function mountWorkspace() {
  stubApi();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AgentRail route={{ screen: "deals" }} />
        <AgentEdge />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const margins = () => document.querySelector(".agentedge");

const switchControl = () =>
  screen.getByRole("switch", { name: LABELS.edgeLight });

async function openPanel(
  user: ReturnType<typeof userEvent.setup>,
  container: HTMLElement,
) {
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
}

// Both the preference and the reading are module state, so a case that
// inherited the last one's would pass for the wrong reason. This is what a
// first-time visitor meets: nothing stored, and the light on.
beforeEach(() => {
  setEdgeLightShown(true);
  window.localStorage.removeItem(EDGE_LIGHT_KEY);
  clearAgentEdge();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the margins, and whether this reader wants them", () => {
  it("lights while the agent works, because nobody has said otherwise", () => {
    render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    expect(document.querySelector("canvas")).not.toBeNull();
  });

  it("draws nothing at all once the light is switched off", () => {
    setEdgeLightShown(false);
    render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    // Not merely dark: the wrapper is gone too, so an unwanted surface is not
    // sitting over every screen paying for a shader nobody can see.
    expect(margins()).toBeNull();
    expect(document.querySelector("canvas")).toBeNull();
  });

  it("relights work that is still in flight when the reader turns it back on", () => {
    setEdgeLightShown(false);
    render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));

    act(() => setEdgeLightShown(true));

    expect(document.querySelector("canvas")).not.toBeNull();
  });

  it("does not hold a fade across the switch", () => {
    // The edge outlives the reading that lit it, by as long as it takes to go
    // out. Switching it off is not the work finishing, and nothing is left
    // mounted to report going dark — so a linger kept across the flip would
    // mount a canvas over an idle window the next time the light came on.
    render(<AgentEdge />);
    act(() => publishAgentEdge({ reading: true, register: "agent" }));
    act(() => setEdgeLightShown(false));
    act(() => publishAgentEdge({ reading: false, register: "agent" }));

    act(() => setEdgeLightShown(true));

    expect(document.querySelector("canvas")).toBeNull();
  });

  it("honours the flip even where the browser refuses to remember it", () => {
    // Storage is refused in some embedded contexts. Persisting is the
    // enhancement; a switch that visibly does nothing when pressed is not a
    // degraded feature, it is a broken control.
    vi.spyOn(window.localStorage, "setItem").mockImplementation(() => {
      throw new Error("storage is not available here");
    });
    render(<AgentEdge />);

    act(() => setEdgeLightShown(false));

    expect(edgeLightShown()).toBe(false);
    expect(margins()).toBeNull();
    vi.restoreAllMocks();
  });

  // A fresh module is what makes this a claim about BOOT: the store resolves
  // once and remembers, so a case that only wrote to storage would be
  // asserting against the answer this file's own cases left in memory.
  it("meets a reader who turned it off last week with the frame still", async () => {
    window.localStorage.setItem(EDGE_LIGHT_KEY, "off");

    vi.resetModules();
    const booted = await import("./agent-edge-preference");

    expect(booted.edgeLightShown()).toBe(false);
  });

  it("meets an install that has never chosen with the light on", async () => {
    window.localStorage.removeItem(EDGE_LIGHT_KEY);

    vi.resetModules();
    const booted = await import("./agent-edge-preference");

    expect(booted.edgeLightShown()).toBe(true);
  });
});

describe("the switch in the agent panel", () => {
  it("stands in the panel's foot, on, because that is what the screen is doing", async () => {
    const user = userEvent.setup();
    const { container } = mountWorkspace();

    await openPanel(user, container);

    const control = switchControl();
    expect(control.getAttribute("aria-checked")).toBe("true");
    expect(control.closest(".arfoot")).not.toBeNull();
  });

  it("takes the margins off the screen when it is flipped, and remembers", async () => {
    const user = userEvent.setup();
    const { container } = mountWorkspace();
    await openPanel(user, container);
    expect(margins()).not.toBeNull();

    await user.click(switchControl());

    await waitFor(() => expect(margins()).toBeNull());
    expect(switchControl().getAttribute("aria-checked")).toBe("false");
    // The next page load is the whole point of a preference: a reader who
    // turned the light off does not want to turn it off again tomorrow.
    expect(window.localStorage.getItem(EDGE_LIGHT_KEY)).toBe("off");
  });

  it("gives them back, so the flip is a preference and not a one-way door", async () => {
    const user = userEvent.setup();
    setEdgeLightShown(false);
    const { container } = mountWorkspace();
    await openPanel(user, container);
    expect(switchControl().getAttribute("aria-checked")).toBe("false");

    await user.click(switchControl());

    await waitFor(() => expect(margins()).not.toBeNull());
    expect(window.localStorage.getItem(EDGE_LIGHT_KEY)).toBe("on");
  });
});
