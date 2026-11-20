/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture, READS_RECORDS } from "../../testing/mefixture";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "#/home";
});

const DEAL_READS = ["/brief", "/magic", "/deal-corrections/pending"];

function refused() {
  return jsonResponse(
    {
      status: 403,
      code: "permission_denied",
      detail: "your role does not grant read on deal",
    },
    403,
  );
}

it("never asks for what needs deal read, and draws no failed section, for a role without it", async () => {
  const { deal: _deal, ...withoutDeals } = READS_RECORDS;
  const calls = stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: withoutDeals })),
    "GET /brief": refused,
    "GET /magic": refused,
    "GET /deal-corrections/pending": refused,
  });
  render(<BriefScreen />);
  // The greeting names the reader, so it shows only once /me has answered.
  expect(await screen.findByText(/Test/)).toBeInTheDocument();
  for (const path of DEAL_READS)
    expect(calls.some((call) => call.path.endsWith(path))).toBe(false);
  expect(screen.queryByText("This section did not load.")).toBeNull();
  expect(screen.queryByText(en["magic.failed"])).toBeNull();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
});

it("asks for them when the role reads deals", async () => {
  const calls = stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: READS_RECORDS })),
  });
  render(<BriefScreen />);
  await waitFor(() => {
    for (const path of DEAL_READS)
      expect(calls.some((call) => call.path.endsWith(path))).toBe(true);
  });
});
