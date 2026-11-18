/** @vitest-environment happy-dom */
import { renderHook } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { useSingleFlight } from "./singleflight";

it("starts one press until it settles, then lets the next through", () => {
  const { result } = renderHook(() => useSingleFlight());
  let settle = () => {};
  const start = vi.fn((settled: () => void) => {
    settle = settled;
  });

  result.current(start);
  result.current(start);
  expect(start).toHaveBeenCalledOnce();

  settle();
  result.current(start);
  expect(start).toHaveBeenCalledTimes(2);
});
