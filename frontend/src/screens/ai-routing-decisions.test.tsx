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
    expect(offered).toEqual(["openrouter_decision", "laya"]);
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "openrouter_decision",
      }),
    );
    await user.type(
      within(lane).getByRole("combobox", { name: "Model" }),
      "jev-classify",
    );
    // The decisions endpoint is OpenRouter's, which it has to be told.
    await user.type(
      within(lane).getByLabelText("Host"),
      "https://openrouter.ai/api/v1",
    );

    const sent = await previewAndSave(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "openrouter_decision",
      model: "jev-classify",
      base_url: "https://openrouter.ai/api/v1",
    });
    // The lanes it sits beside are sent untouched.
    expect(sent?.embeddings.model).toBe("gemini-embedding-001");
  });

  // A model and a host name one adapter's endpoint; carried onto another
  // adapter they would point Laya at OpenRouter. A provider switch clears both.
  it("clears the model and host when the decision provider changes", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: {
        provider: "openrouter_decision",
        model: "jev-classify",
        base_url: "https://openrouter.ai/api",
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("jev-classify");

    const lane = await openLane(user, "ai-routing-decisions");
    await user.click(within(lane).getByRole("combobox", { name: "Provider" }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", { name: "laya" }),
    );

    const sent = await previewAndSave(user, backend);
    expect(sent?.decisions).toEqual({ provider: "laya", model: "" });
  });

  // A decision is billed on its input alone, as an embedding is: the row
  // prints one figure, and a blank output price is the sheet being right
  // rather than a reason to call the binding unpriced.
  it("prices a decision model on its input alone", async () => {
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "openrouter_decision", model: "jev-classify" },
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
      decisions: { provider: "laya", model: "laya-small" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("laya-small");

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
