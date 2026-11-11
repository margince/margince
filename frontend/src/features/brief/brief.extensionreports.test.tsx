/** @vitest-environment happy-dom */
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  type Call,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "#/home";
});

const reportReads = (calls: readonly Call[]) =>
  calls.filter((call) => call.path.includes("/extension-reports"));

it("asks for the extension reports only once the day's own reads have answered", async () => {
  let answerMagic: (response: Response) => void = () => undefined;
  const calls = stubApi({
    "GET /magic": () =>
      new Promise<Response>((resolve) => {
        answerMagic = resolve;
      }),
  });
  render(<BriefScreen />);
  await waitFor(() =>
    expect(calls.some((call) => call.path.includes("/worklist"))).toBe(true),
  );
  expect(reportReads(calls)).toEqual([]);

  answerMagic(jsonResponse({ title: "Unavailable" }, 503));
  await waitFor(() => expect(reportReads(calls)).not.toEqual([]));
});
