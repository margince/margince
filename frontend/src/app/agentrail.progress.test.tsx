/** @vitest-environment happy-dom */
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  block,
  jsonResponse,
  render,
  stubAgentRailApi,
} from "./agentrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const SITE_READ = (over: Readonly<Record<string, unknown>> = {}) => ({
  id: "019f7e65-fbf7-7114-b114-40af4af63a02",
  kind: "site_read",
  state: "running",
  started_at: new Date(Date.now() - 60_000).toISOString(),
  ...over,
});

const mountWith = (run: unknown) => {
  stubAgentRailApi({
    agentActivity: () =>
      jsonResponse({ running: [run], recent: [], faults: [] }),
  });
  return render();
};

const ringShare = (container: HTMLElement): string | null =>
  container
    .querySelector(".core-progress-value")
    ?.getAttribute("stroke-dasharray") ?? null;

const settlesOnIngest = (container: HTMLElement) =>
  waitFor(() =>
    expect(block(container).getAttribute("data-core-state")).toBe("ingest"),
  );

describe("the ring around a run the feed names", () => {
  it("stays unmeasured for a run that reports no count", async () => {
    const { container } = mountWith(SITE_READ());
    await settlesOnIngest(container);
    expect(container.querySelector(".core-progress")).toBeNull();
  });

  it("stays unmeasured for a count without a total", async () => {
    const { container } = mountWith(SITE_READ({ processed: 3 }));
    await settlesOnIngest(container);
    expect(container.querySelector(".core-progress")).toBeNull();
  });

  it("draws the measured arc when both counts are present", async () => {
    const { container } = mountWith(SITE_READ({ processed: 3, total: 12 }));
    await settlesOnIngest(container);
    await waitFor(() => expect(ringShare(container)).toBe("25 100"));
  });

  it("clamps a count that has run past its total", async () => {
    const { container } = mountWith(SITE_READ({ processed: 15, total: 12 }));
    await settlesOnIngest(container);
    await waitFor(() => expect(ringShare(container)).toBe("100 100"));
  });

  it("draws no ring for null counts the server sends while unknown", async () => {
    const { container } = mountWith(
      SITE_READ({ processed: null, total: null }),
    );
    await settlesOnIngest(container);
    expect(container.querySelector(".core-progress")).toBeNull();
  });
});
