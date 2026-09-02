// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { useAgentFault } from "./agent-fault";

type AiActivityItem = components["schemas"]["AiActivityItem"];

const SEEN_KEY = "margince.agent.faults-seen";

/** One settled run, newest-first order being the caller's business. */
function settled(over: Partial<AiActivityItem>): AiActivityItem {
  return {
    id: "019f7e65-fbf7-7114-b114-40af4af63d01",
    kind: "morning_brief",
    state: "failed",
    started_at: "2026-08-21T05:00:00Z",
    finished_at: "2026-08-21T05:00:05Z",
    ...over,
  };
}

const SCHEDULED = settled({
  id: "019f7e65-fbf7-7114-b114-40af4af63d02",
  kind: "morning_brief",
});
const ATTENDED = settled({
  id: "019f7e65-fbf7-7114-b114-40af4af63d01",
  kind: "draft_reply",
});

describe("useAgentFault", () => {
  beforeEach(() => {
    window.localStorage.removeItem(SEEN_KEY);
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  // The orb shows the NEWEST fault, and a scheduled one that failed after an
  // attended one stands in front of it. The attended fault's clock has to run
  // anyway: armed only for the front of the list, it would wait behind the
  // brief for hours and then flash the moment the brief was acknowledged —
  // over a draft the reader retried that morning.
  it("expires an attended fault standing behind a scheduled one", () => {
    let recent: readonly AiActivityItem[] = [SCHEDULED, ATTENDED];
    const { result, rerender } = renderHook(() => useAgentFault(recent));
    expect(result.current.fault?.item.id).toBe(SCHEDULED.id);

    act(() => {
      vi.advanceTimersByTime(8_100);
    });
    // The scheduled fault still holds: nobody has looked yet.
    expect(result.current.fault?.item.id).toBe(SCHEDULED.id);

    // The brief leaves the feed; the draft's flash is already spent, so it
    // does not take the orb over.
    recent = [ATTENDED];
    rerender();
    expect(result.current.fault).toBeNull();
  });

  // Each attended fault flashes from the moment IT was first seen. A second
  // one arriving must not restart the first one's clock.
  it("keeps one attended fault's clock when another arrives", () => {
    const later = settled({
      id: "019f7e65-fbf7-7114-b114-40af4af63d03",
      kind: "summarize",
    });
    let recent: readonly AiActivityItem[] = [ATTENDED];
    const { result, rerender } = renderHook(() => useAgentFault(recent));
    act(() => {
      vi.advanceTimersByTime(6_000);
    });
    recent = [later, ATTENDED];
    rerender();
    expect(result.current.fault?.item.id).toBe(later.id);

    // Eight seconds after the OLDER one was first seen it is spent, and the
    // newer one still holds the orb with six seconds of its own to run — a
    // clock restarted by its arrival would keep it lit two seconds longer.
    act(() => {
      vi.advanceTimersByTime(7_900);
    });
    expect(result.current.fault?.item.id).toBe(later.id);
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current.fault).toBeNull();
  });

  // The rule the module was built for, unchanged: the brief that failed at
  // four in the morning holds until somebody opens the panel.
  it("holds a scheduled fault until acknowledged", () => {
    const { result } = renderHook(() => useAgentFault([SCHEDULED]));
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(result.current.fault?.item.id).toBe(SCHEDULED.id);
    act(() => {
      result.current.acknowledge();
    });
    expect(result.current.fault).toBeNull();
  });
});
