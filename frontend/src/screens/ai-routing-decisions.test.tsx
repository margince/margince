/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AiRoutingCard } from "./ai-routing";
import {
  BOUND,
  backendFor,
  openLane,
  previewAndSave,
  ROUTING_EDITOR,
  render,
} from "./ai-routing.testkit";
import { OPENROUTER_DECISION_PRESET } from "./ai-routing-fields";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the decision model lane", () => {
  // Absent is a real state: no task asks a decision model first. The row says
  // so and offers to add one; it never draws an empty binding the server would
  // refuse, and it offers only the adapters that answer a decision.
  it("shows a decision model row and saves it", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const absent = await screen.findByTestId("ai-routing-decisions");
    expect(within(absent).getByText(/no decision model/i)).toBeInTheDocument();
    await user.click(
      within(absent).getByRole("button", { name: "Add decision model" }),
    );
    // Adding opens the lane's fields; the row is now a binding like any other.
    const lane = screen.getByTestId("ai-routing-decisions");

    const provider = within(lane).getByRole("combobox", { name: "Provider" });
    await user.click(provider);
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["jev", "jev_compatible"]);
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "jev_compatible",
      }),
    );
    await user.type(
      within(lane).getByRole("combobox", { name: "Model" }),
      "jev-classify",
    );
    // Any server on the Jev wire, so the endpoint is the binding: the full
    // URL, sent as typed.
    await user.type(
      within(lane).getByLabelText("Host"),
      "http://127.0.0.1:8767/v1/systemone",
    );

    const sent = await previewAndSave(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "jev_compatible",
      model: "jev-classify",
      base_url: "http://127.0.0.1:8767/v1/systemone",
    });
    // The lanes it sits beside are sent untouched.
    expect(sent?.embeddings.model).toBe("gemini-embedding-001");
  });

  // OpenRouter's endpoint is a URL nobody remembers, so jev_compatible offers
  // it in one press — the endpoint and the model certified there — and names
  // the key it needs, which the preset cannot fill.
  it("fills OpenRouter's endpoint and model from the preset and saves them", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev_compatible", model: "" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    await screen.findByTestId("ai-routing-decisions");
    const lane = await openLane(user, "ai-routing-decisions");
    expect(
      within(lane).getByText(
        /JEV_COMPATIBLE_API_KEY takes your OpenRouter key/,
      ),
    ).toBeInTheDocument();
    await user.click(
      within(lane).getByRole("button", { name: "Use OpenRouter" }),
    );
    expect(within(lane).getByLabelText("Host")).toHaveValue(
      OPENROUTER_DECISION_PRESET.base_url,
    );
    expect(within(lane).getByRole("combobox", { name: "Model" })).toHaveValue(
      OPENROUTER_DECISION_PRESET.model,
    );

    const sent = await previewAndSave(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "jev_compatible",
      model: "typesafe/jev-1.13",
      base_url: "https://openrouter.ai/api/alpha/decisions",
    });
  });

  // The official API has an endpoint of its own; the preset is for the
  // adapter that needs one named.
  it("offers the OpenRouter preset only on jev_compatible", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev", model: "jev-1.13.0" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    await screen.findByTestId("ai-routing-decisions");
    const lane = await openLane(user, "ai-routing-decisions");
    expect(
      within(lane).queryByRole("button", { name: "Use OpenRouter" }),
    ).toBeNull();
  });

  // A model and a host name one adapter's endpoint; carried onto another
  // adapter they would point TypeSafe's own API at OpenRouter. A provider
  // switch clears both.
  it("clears the model and host when the decision provider changes", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: {
        provider: "jev_compatible",
        model: "jev-classify",
        base_url: "https://openrouter.ai/api/alpha/decisions",
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("jev-classify");

    const lane = await openLane(user, "ai-routing-decisions");
    await user.click(within(lane).getByRole("combobox", { name: "Provider" }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", { name: "jev" }),
    );

    const sent = await previewAndSave(user, backend);
    expect(sent?.decisions).toEqual({ provider: "jev", model: "" });
  });

  // A decision is billed on its input alone, as an embedding is: the row
  // prints one figure, and a blank output price is the sheet being right
  // rather than a reason to call the binding unpriced.
  it("prices a decision model on its input alone", async () => {
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev_compatible", model: "jev-classify" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("jev-classify");

    const lane = screen.getByTestId("ai-routing-decisions");
    expect(
      await within(lane).findByText("Input US$0.40 per 1M tokens"),
    ).toBeInTheDocument();
    expect(within(lane).queryByText("Unpriced")).toBeNull();
  });

  it("removing the decision model drops the key", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev", model: "jev-latest" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("jev-latest");

    const lane = await openLane(user, "ai-routing-decisions");
    await user.click(
      within(lane).getByRole("button", { name: "Remove decision model" }),
    );

    const sent = await previewAndSave(user, backend);
    expect(sent).not.toHaveProperty("decisions");
    expect(sent?.tiers.premium.model).toBe("gemini-3.5-flash");
  });
});

it("keeps the draft revision after a background refresh and refuses a conflicting preview", async () => {
  const user = userEvent.setup({ delay: null });
  const backend = backendFor(ROUTING_EDITOR);
  vi.stubGlobal("fetch", backend.fetchMock);
  const { client } = render(<AiRoutingCard />);
  await screen.findByText("gemini-3.5-flash");
  const lane = await openLane(user, "ai-routing-tier-premium");
  const model = within(lane).getByRole("combobox", { name: "Model" });
  await user.clear(model);
  await user.type(model, "my-draft-model");
  backend.externalChange();
  await client.invalidateQueries({ queryKey: ["ai-routing"] });
  await user.click(screen.getByRole("button", { name: /preview effects/i }));
  await screen.findByText(/model bindings changed while you were editing/i);
  expect(model).toHaveValue("my-draft-model");
  expect(screen.getByRole("button", { name: /save routing/i })).toBeDisabled();
  expect(backend.getCapturedPut()).toBeNull();
});

it("keeps a manually opened advanced section open after closing a tier editor", async () => {
  const backend = backendFor(ROUTING_EDITOR, BOUND);
  vi.stubGlobal("fetch", backend.fetchMock);
  render(<AiRoutingCard />);
  const user = userEvent.setup({ delay: null });
  const summary = await screen.findByText("Advanced: shared model bindings");
  await user.click(summary);
  const details = summary.closest("details");
  expect(details).toHaveAttribute("open");
  const tier = await openLane(user, "ai-routing-tier-premium");
  await user.click(within(tier).getByRole("button", { name: "Done" }));
  expect(details).toHaveAttribute("open");
});

// Which edits take a binding's broker preferences off. Only a move to another
// provider or another address does: a new model at the same OpenRouter address
// is still the binding the preferences were written for.
