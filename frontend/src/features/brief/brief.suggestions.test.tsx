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
