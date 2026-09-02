/** @vitest-environment jsdom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  beginModelCall,
  endModelCall,
  modelCallsInFlight,
  useModelCallsInFlight,
} from "./model-inflight";

afterEach(() => {
  cleanup();
  while (modelCallsInFlight() > 0) {
    endModelCall();
  }
});

// Every change is published, not only the crossings of zero. The rail reads
// the count as a boolean, but the feed is refetched on each edge of a call, so
// two calls overlapping would otherwise have their inner edges — the second
// leaving, the first answering — pass without a read.
it("reports every step of overlapping calls to its subscribers", () => {
  const { result } = renderHook(() => useModelCallsInFlight());
  const seen = [result.current];
  for (const step of [
    beginModelCall,
    beginModelCall,
    endModelCall,
    endModelCall,
  ]) {
    act(step);
    seen.push(result.current);
  }
  expect(seen).toEqual([0, 1, 2, 1, 0]);
});

// Nothing began, so nothing can end. A count allowed below zero would read as
// false in every consumer and hide the NEXT real call instead of the bug.
it("never goes below zero", () => {
  endModelCall();
  expect(modelCallsInFlight()).toBe(0);
});
