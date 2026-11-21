/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useSavedDraft } from "./usesaveddraft";

type Request = { resolve: () => void; reject: () => void };

function setUp() {
  const requests: Request[] = [];
  const hook = renderHook(
    ({ saved }) =>
      useSavedDraft({
        saved,
        toDraft: (value: string) => value,
        submit: (draft) =>
          draft === saved
            ? undefined
            : new Promise<void>((resolve, reject) => {
                requests.push({ resolve, reject: () => reject(new Error()) });
              }),
      }),
    { initialProps: { saved: "a" } },
  );
  return { requests, ...hook };
}

describe("useSavedDraft", () => {
  it("keeps a newer draft that was already sent when an earlier answer arrives", async () => {
    const { result, rerender, requests } = setUp();
    act(() => result.current.edit("ab"));
    act(() => result.current.leave());
    act(() => result.current.edit("abc"));
    act(() => result.current.leave());

    rerender({ saved: "ab" });
    await act(async () => requests[0].resolve());
    expect(result.current.draft).toBe("abc");

    await act(async () => requests[1].reject());
    expect(result.current.draft).toBe("abc");
    expect(result.current.unsaved).toBe(true);
  });

  it("takes the saved value once every sent draft is answered", async () => {
    const { result, rerender, requests } = setUp();
    act(() => result.current.edit("ab"));
    act(() => result.current.leave());
    rerender({ saved: "AB" });
    await act(async () => requests[0].resolve());
    expect(result.current.draft).toBe("AB");
    expect(result.current.unsaved).toBe(false);
  });

  it("sends text edited back to the old value once the earlier answer moves the saved value", async () => {
    const { result, rerender, requests } = setUp();
    act(() => result.current.edit("ab"));
    act(() => result.current.leave());
    act(() => result.current.edit("a"));
    act(() => result.current.leave());
    expect(requests).toHaveLength(1);

    rerender({ saved: "ab" });
    await act(async () => requests[0].resolve());
    expect(requests).toHaveLength(2);
  });

  it("drops a failed draft for the saved value when the reader restores it", async () => {
    const { result, requests } = setUp();
    act(() => result.current.edit("ab"));
    act(() => result.current.leave());
    await act(async () => requests[0].reject());
    act(() => result.current.restore());
    expect(result.current.draft).toBe("a");
    expect(result.current.unsaved).toBe(false);
  });
});
