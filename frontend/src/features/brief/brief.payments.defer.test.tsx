/** @vitest-environment happy-dom */
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  type Call,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "#/home";
});

const paymentReads = (calls: readonly Call[]) =>
  calls.filter((call) => call.path.includes("/finance/customer-payments"));

it("asks for customer payments only once the day's own reads have answered", async () => {
  let answerMagic: (response: Response) => void = () => undefined;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: { company: ["read"], deal: ["read"], finance: ["read"] },
        }),
      ),
    "GET /magic": () =>
      new Promise<Response>((resolve) => {
        answerMagic = resolve;
      }),
  });
  render(<BriefScreen />);
  await waitFor(() =>
    expect(calls.some((call) => call.path.includes("/worklist"))).toBe(true),
  );
  expect(paymentReads(calls)).toEqual([]);

  answerMagic(jsonResponse({ title: "Unavailable" }, 503));
  await waitFor(() => expect(paymentReads(calls)).not.toEqual([]));
  expect(calls.some((call) => call.path.includes("/extension-reports"))).toBe(
    false,
  );
});
