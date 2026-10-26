/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { ranked } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefSuggestions } from "./brief.suggestions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("opens the stored run without turning suggestions into work or loading names while closed", async () => {
  const user = userEvent.setup();
  const calls = stubApi({
    "GET /brief": () =>
      jsonResponse({
        ...ranked,
        items: [
          {
            ...ranked.items[0],
            finding: "Buyer asked to revisit rollout in November.",
          },
        ],
      }),
    "GET /deals/d-1": () => jsonResponse({ id: "d-1", name: "Acme rollout" }),
  });
  const { container } = render(<BriefSuggestions />);
  expect(calls).toHaveLength(0);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  expect(
    (await screen.findByRole("link", { name: "Acme rollout" })).getAttribute(
      "href",
    ),
  ).toBe("#/deals/d-1");
  expect(screen.getByText(/Buyer asked to revisit rollout/)).toBeTruthy();
  expect(container.textContent).not.toMatch(
    /0\.74|winnability|Ranked|Done|Mark handled/,
  );
  expect(calls.filter((call) => call.method !== "GET")).toHaveLength(0);
});

it.each([null, { ...ranked, items: [] }])(
  "shows an absent or empty run honestly",
  async (run) => {
    const user = userEvent.setup();
    stubApi({
      "GET /brief": () => (run ? jsonResponse(run) : jsonResponse({}, 404)),
    });
    render(<BriefSuggestions />);
    await user.click(screen.getByText(en["brief.suggestions.review"]));
    expect(await screen.findByText(en["brief.suggestions.empty"])).toBeTruthy();
  },
);

it("keeps a failed read distinct from an empty run", async () => {
  const user = userEvent.setup();
  stubApi({ "GET /brief": () => jsonResponse({ title: "Unavailable" }, 503) });
  render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  expect(screen.queryByText(en["brief.suggestions.empty"])).toBeNull();
});

it("never displays stored findings for a deal the current read refuses", async () => {
  const user = userEvent.setup();
  const calls = stubApi({
    "GET /brief": () =>
      jsonResponse({
        ...ranked,
        items: [{ ...ranked.items[0], finding: "Hidden buyer context" }],
      }),
    "GET /deals/d-1": () => jsonResponse({}, 404),
  });
  const { container } = render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  await waitFor(() =>
    expect(calls.some((call) => call.path === "/deals/d-1")).toBe(true),
  );
  expect(container.textContent).not.toContain("Hidden buyer context");
  expect(container.textContent).not.toContain("d-1");
  expect(screen.queryByRole("link")).toBeNull();
});

it("offers retry for a failed name read and restores the deal only after success", async () => {
  const user = userEvent.setup();
  let fails = true;
  stubApi({
    "GET /brief": () =>
      jsonResponse({
        ...ranked,
        items: [
          { ...ranked.items[0], finding: "Buyer requested a rollout plan." },
        ],
      }),
    "GET /deals/d-1": () =>
      fails
        ? jsonResponse({}, 503)
        : jsonResponse({ id: "d-1", name: "Acme rollout" }),
  });
  render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  expect(await screen.findByText(en["state.failed"])).toBeTruthy();
  expect(screen.queryByText(/Buyer requested/)).toBeNull();
  expect(screen.queryByText(en["brief.suggestions.empty"])).toBeNull();
  fails = false;
  await user.click(screen.getByRole("button", { name: en["state.retry"] }));
  expect(
    await screen.findByRole("link", { name: "Acme rollout" }),
  ).toBeTruthy();
  expect(screen.getByText(/Buyer requested/)).toBeTruthy();
  expect(screen.queryByText(en["state.failed"])).toBeNull();
});

// The overnight pass writes `annotated_at` on the run whether or not it found
// anything; `finding` is per deal. Those two facts are three different states,
// and only one of them is a suggestion with something to read.
const DEALS = {
  "GET /deals/d-1": () => jsonResponse({ id: "d-1", name: "Acme rollout" }),
  "GET /deals/d-2": () => jsonResponse({ id: "d-2", name: "Globex renewal" }),
};
const twoDeals = (overrides: Readonly<Record<string, unknown>>) => ({
  ...ranked,
  narrative: null,
  items: [
    { ...ranked.items[0], id: "i-1", deal_id: "d-1", rank: 1, finding: null },
    { ...ranked.items[0], id: "i-2", deal_id: "d-2", rank: 2, finding: null },
  ],
  ...overrides,
});

it("says no assessment was written yet when no pass annotated the run, and draws no empty recommendation", async () => {
  const user = userEvent.setup();
  stubApi({
    "GET /brief": () => jsonResponse(twoDeals({ annotated_at: null })),
    ...DEALS,
  });
  render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  expect(
    await screen.findByText(en["brief.suggestions.notAnnotated"]),
  ).toBeTruthy();
  expect(screen.queryByText(en["brief.suggestions.nothingToAdd"])).toBeNull();
  // Both ranked deals stay reachable while the annotation is pending.
  expect(await screen.findByRole("link", { name: "Acme rollout" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "Globex renewal" })).toBeTruthy();
  // And none of them is drawn as a recommendation heading with nothing under it.
  expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(0);
  expect(
    screen.queryByText(new RegExp(`^${en["worklist.verdict.believes"]} `)),
  ).toBeNull();
});

it("says the pass had nothing to add when it ran and wrote no finding", async () => {
  const user = userEvent.setup();
  stubApi({
    "GET /brief": () =>
      jsonResponse(twoDeals({ annotated_at: "2026-10-26T01:10:00Z" })),
    ...DEALS,
  });
  render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  expect(
    await screen.findByText(en["brief.suggestions.nothingToAdd"]),
  ).toBeTruthy();
  expect(screen.queryByText(en["brief.suggestions.notAnnotated"])).toBeNull();
  expect(await screen.findByRole("link", { name: "Globex renewal" })).toBeTruthy();
  expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(0);
});

it("shows a real finding as a suggestion and lists the deal the pass said nothing about under the quiet line", async () => {
  const user = userEvent.setup();
  stubApi({
    "GET /brief": () =>
      jsonResponse(
        twoDeals({
          annotated_at: "2026-10-26T01:10:00Z",
          items: [
            {
              ...ranked.items[0],
              id: "i-1",
              deal_id: "d-1",
              rank: 1,
              finding: "Buyer asked for the security review before signing.",
            },
            { ...ranked.items[0], id: "i-2", deal_id: "d-2", rank: 2, finding: null },
          ],
        }),
      ),
    ...DEALS,
  });
  render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  const heading = await screen.findByRole("heading", { level: 3 });
  expect(heading.textContent).toBe("Acme rollout");
  expect(screen.getByText(/security review before signing/)).toBeTruthy();
  expect(screen.getByText(en["brief.suggestions.nothingToAdd"])).toBeTruthy();
  expect(await screen.findByRole("link", { name: "Globex renewal" })).toBeTruthy();
  expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(1);
});

it("drops a deal the reader may no longer open from the quiet list as well", async () => {
  const user = userEvent.setup();
  stubApi({
    "GET /brief": () => jsonResponse(twoDeals({ annotated_at: null })),
    "GET /deals/d-1": () => jsonResponse({}, 404),
    "GET /deals/d-2": DEALS["GET /deals/d-2"],
  });
  const { container } = render(<BriefSuggestions />);
  await user.click(screen.getByText(en["brief.suggestions.review"]));
  expect(await screen.findByRole("link", { name: "Globex renewal" })).toBeTruthy();
  await waitFor(() => expect(screen.getAllByRole("link")).toHaveLength(1));
  expect(container.textContent).not.toContain("d-1");
});
