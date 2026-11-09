/** @vitest-environment happy-dom */
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MarginceCoreScene } from "./margince-core";

const renderer = vi.hoisted(() => {
  let loaded: (() => void) | undefined;
  const ready = new Promise<void>((resolve) => {
    loaded = resolve;
  });
  return { ready, load: () => loaded?.(), create: vi.fn(() => null) };
});
vi.mock("./margince-core-gl", async () => {
  await renderer.ready;
  return { createCoreRenderer: renderer.create };
});
beforeEach(() => renderer.create.mockClear());
afterEach(cleanup);

it("does not allocate a renderer for a canvas unmounted while its code loads", async () => {
  const { unmount } = render(<MarginceCoreScene state="working" />);
  expect(renderer.create).not.toHaveBeenCalled();
  unmount();
  await act(async () => {
    renderer.load();
    await renderer.ready;
  });
  expect(renderer.create).not.toHaveBeenCalled();
});

it("starts the loaded renderer for the current canvas and keeps the static fallback on refusal", async () => {
  renderer.load();
  const { container } = render(<MarginceCoreScene state="idle" />);
  await waitFor(() => expect(renderer.create).toHaveBeenCalledOnce());
  expect(renderer.create).toHaveBeenCalledWith(
    container.querySelector("canvas"),
  );
  expect(container.querySelector(".core-still")).not.toBeNull();
});
