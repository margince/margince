/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { AiRoutingCard } from "./ai-routing";
import {
  BOUND,
  backendFor,
  CHANGED_ELSEWHERE,
  openEditor,
  ROUTING_EDITOR,
  render,
  saveEditor,
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
      within(absent).getByRole("button", {
        name: englishCopy["aiRouting.decisions.add"],
      }),
    );
    // Adding opens the lane's editor, named for the add, with nothing yet to
    // remove.
    const lane = await screen.findByRole("dialog", {
      name: englishCopy["aiRouting.decisions.add"],
    });
    expect(
      within(lane).queryByRole("button", {
        name: englishCopy["aiRouting.decisions.remove"],
      }),
    ).toBeNull();

    const provider = within(lane).getByRole("combobox", { name: "Provider" });
    await user.click(provider);
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual([
      englishCopy["aiProviders.name.jev"],
      englishCopy["aiProviders.name.jevCompatible"],
      englishCopy["aiProviders.name.geminiDecision"],
    ]);
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: englishCopy["aiProviders.name.jevCompatible"],
      }),
    );
    await user.type(
      within(lane).getByRole("combobox", { name: "Model" }),
      "jev-classify",
    );
    // Any server on the Jev wire: its endpoint is the provider's, set on the
    // provider's sheet, so the lane names none and says where to set it.
    expect(within(lane).queryByLabelText("Host")).toBeNull();
    expect(
      within(lane).getByText(/Jev-compatible has no host yet/),
    ).toBeInTheDocument();

    const sent = await saveEditor(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "jev_compatible",
      model: "jev-classify",
    });
    // The lanes it sits beside are sent untouched.
    expect(sent?.embeddings.model).toBe("gemini-embedding-001");
  });

  // Google's Decisions API has its own endpoint and sends the Gemini key. So
  // choosing it needs no host, and its one model is offered.
  it("binds Google's decision model with no host to set", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openEditor(
      user,
      "ai-routing-decisions",
      /add decision model/i,
    );
    await user.click(within(lane).getByRole("combobox", { name: "Provider" }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: englishCopy["aiProviders.name.geminiDecision"],
      }),
    );
    await user.click(within(lane).getByRole("combobox", { name: "Model" }));
    await user.click(
      await within(screen.getByRole("listbox")).findByRole("option", {
        name: "gemini-decision-1",
      }),
    );
    expect(within(lane).queryByText(/has no host yet/)).toBeNull();

    const sent = await saveEditor(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "gemini_decision",
      model: "gemini-decision-1",
    });
  });

  // gemini_decision has no key row: it sends the gemini key, which the gemini
  // row names in `sent_by`. Without that key it is missing one, as any other
  // provider is.
  const unkeyedGemini = [
    {
      provider: "gemini",
      configured: false,
      env_var: "GEMINI_API_KEY",
      usable: false,
      sent_by: ["gemini_decision"],
    },
    {
      provider: "jev",
      configured: true,
      env_var: "TYPESAFE_API_KEY",
      usable: true,
    },
  ];

  it("says gemini_decision's key is missing when the gemini row holds none", async () => {
    const user = userEvent.setup();
    const backend = backendFor(
      ROUTING_EDITOR,
      {
        ...BOUND,
        decisions: { provider: "gemini_decision", model: "gemini-decision-1" },
      },
      { providerKeys: unkeyedGemini },
    );
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openEditor(user, "ai-routing-decisions");
    expect(
      within(lane).getByText(englishCopy["aiRouting.keyMissing"]),
    ).toBeInTheDocument();
  });

  it("offers gemini_decision to a new binding only once the gemini key is held", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, BOUND, {
      providerKeys: unkeyedGemini,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openEditor(
      user,
      "ai-routing-decisions",
      /add decision model/i,
    );
    await user.click(within(lane).getByRole("combobox", { name: "Provider" }));
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).not.toContain(
      englishCopy["aiProviders.name.geminiDecision"],
    );
  });

  // An installation that reaches decisions only through OpenRouter holds no
  // TypeSafe key, so adding opens on the adapter it CAN use rather than on a
  // refused one.
  it("adds a decision model on a provider this installation can reach", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, BOUND, {
      providerKeys: [
        {
          provider: "gemini",
          configured: true,
          env_var: "GEMINI_API_KEY",
          usable: true,
        },
        {
          provider: "jev",
          configured: false,
          env_var: "TYPESAFE_API_KEY",
          usable: false,
        },
        {
          provider: "jev_compatible",
          configured: false,
          env_var: "JEV_COMPATIBLE_API_KEY",
          usable: true,
          optional: true,
        },
      ],
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const dialog = await openEditor(
      user,
      "ai-routing-decisions",
      /add decision model/i,
    );
    expect(
      within(dialog).getByRole("combobox", { name: "Provider" }),
    ).toHaveTextContent(englishCopy["aiProviders.name.jevCompatible"]);
    expect(
      within(dialog).queryByText(englishCopy["aiRouting.keyMissing"]),
    ).toBeNull();
  });

  // OpenRouter's endpoint is a URL nobody remembers, so jev_compatible offers
  // it in one press — the endpoint and the model certified there — and names
  // the key it needs, which the preset cannot fill. With no endpoint on the
  // provider yet, the endpoint travels on the lane and the server lifts it.
  it("fills OpenRouter's endpoint and model from the preset and saves them", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev_compatible", model: "" },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    await screen.findByTestId("ai-routing-decisions");
    const lane = await openEditor(user, "ai-routing-decisions");
    expect(
      within(lane).getByText(/The key goes in JEV_COMPATIBLE_API_KEY/),
    ).toBeInTheDocument();
    await user.click(
      within(lane).getByRole("button", {
        name: englishCopy["aiRouting.decisions.preset.openrouter"],
      }),
    );
    expect(within(lane).getByRole("combobox", { name: "Model" })).toHaveValue(
      OPENROUTER_DECISION_PRESET.model,
    );

    const sent = await saveEditor(user, backend);
    expect(sent?.decisions).toEqual({
      provider: "jev_compatible",
      model: "typesafe/jev-1.13",
      base_url: englishCopy["aiRouting.baseUrl.placeholder.jevCompatible"],
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
    const lane = await openEditor(user, "ai-routing-decisions");
    expect(
      within(lane).queryByRole("button", {
        name: englishCopy["aiRouting.decisions.preset.openrouter"],
      }),
    ).toBeNull();
  });

  // A provider already pointed at another decision server keeps it, so a
  // preset that would set OpenRouter's model there is not offered.
  it("offers no OpenRouter preset when the provider is pointed elsewhere", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      decisions: { provider: "jev_compatible", model: "jev-classify" },
      providers: {
        jev_compatible: { base_url: "http://127.0.0.1:8767/v1/systemone" },
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    await screen.findByTestId("ai-routing-decisions");
    const lane = await openEditor(user, "ai-routing-decisions");
    expect(
      within(lane).queryByRole("button", {
        name: englishCopy["aiRouting.decisions.preset.openrouter"],
      }),
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

    const lane = await openEditor(user, "ai-routing-decisions");
    await user.click(within(lane).getByRole("combobox", { name: "Provider" }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: englishCopy["aiProviders.name.jev"],
      }),
    );

    const model = within(lane).getByRole("combobox", { name: "Model" });
    expect(model).toHaveValue("");
    // An empty model is not a binding, so Save waits for one.
    expect(
      within(lane).getByRole("button", { name: /save binding/i }),
    ).toBeDisabled();
    await user.type(model, "jev-1.13.0");

    const sent = await saveEditor(user, backend);
    expect(sent?.decisions).toEqual({ provider: "jev", model: "jev-1.13.0" });
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

    const lane = await openEditor(user, "ai-routing-decisions");
    await user.click(
      within(lane).getByRole("button", {
        name: englishCopy["aiRouting.decisions.remove"],
      }),
    );
    await waitFor(() => expect(backend.getPutCount()).toBe(1));
    const sent = backend.getCapturedPut();
    expect(sent).not.toHaveProperty("decisions");
    expect(sent?.tiers.premium.model).toBe("gemini-3.5-flash");
  });
});

describe("saving one binding", () => {
  // The editor remembers the binding it opened on. A colleague who re-pointed
  // THIS lane in the meantime is caught before anything is written, and the
  // reader's edit stays on screen for a deliberate second Save.
  it("refuses to overwrite a lane someone else changed since it opened", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-premium");
    const model = within(dialog).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "my-draft-model");

    backend.externalChange();
    await user.click(
      within(dialog).getByRole("button", { name: /save binding/i }),
    );

    expect(
      await within(dialog).findByText(
        /model bindings changed while you were editing/i,
      ),
    ).toBeTruthy();
    expect(backend.getPutCount()).toBe(0);
    expect(model).toHaveValue("my-draft-model");

    // Saving again is the reader choosing to replace the colleague's binding.
    const sent = await saveEditor(user, backend);
    expect(sent?.tiers.premium.model).toBe("my-draft-model");
  });

  // An edit to ANOTHER lane is not a conflict: the save lands on the latest
  // document, so the colleague's change travels with it rather than being
  // reverted by a stale copy.
  it("keeps a colleague's edit to another lane", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-cheap_cloud");
    const model = within(dialog).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-3.5-flash");

    backend.externalChange(CHANGED_ELSEWHERE);
    const sent = await saveEditor(user, backend);

    expect(sent?.tiers.cheap_cloud.model).toBe("gemini-3.5-flash");
    expect(sent?.tiers.premium.model).toBe("gemini-3.1-pro-preview");
  });

  // The window the editor's own re-read cannot close: a write that lands
  // between that read and the PUT. The server's If-Match catches it, and the
  // editor answers the 409 the way it answers a caught stale base.
  it("answers a server conflict with the callout, not a dead form", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-premium");
    const model = within(dialog).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "my-draft-model");

    backend.raceNextPut();
    await user.click(
      within(dialog).getByRole("button", { name: /save binding/i }),
    );

    expect(
      await within(dialog).findByText(
        /model bindings changed while you were editing/i,
      ),
    ).toBeTruthy();
    expect(within(dialog).queryByText(/routing not saved/i)).toBeNull();
    const sent = await saveEditor(user, backend);
    expect(sent?.tiers.premium.model).toBe("my-draft-model");
  });

  // A race on ANOTHER lane is no conflict: the 409 is retried once, and the
  // re-read shows this lane untouched, so the edit lands with theirs.
  it("retries a server conflict that came from another lane", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-cheap_cloud");
    const model = within(dialog).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-3.5-flash");

    backend.raceNextPut();
    const sent = await saveEditor(user, backend);

    expect(sent?.tiers.cheap_cloud.model).toBe("gemini-3.5-flash");
    expect(sent?.tiers.premium.model).toBe("gemini-3.1-pro-preview");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  // The vendor's list is a hint, not a permitted set: an id it does not name
  // is said so, and still saves.
  it("hints at a model the vendor does not list, and saves it anyway", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-premium");
    const model = within(dialog).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-9-imaginary");

    expect(
      await within(dialog).findByText(
        /not in Google Gemini’s published model list/i,
      ),
    ).toBeTruthy();
    const sent = await saveEditor(user, backend);
    expect(sent?.tiers.premium.model).toBe("gemini-9-imaginary");
  });

  it("closes without writing on Cancel", async () => {
    const user = userEvent.setup({ delay: null });
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");
    const dialog = await openEditor(user, "ai-routing-tier-premium");
    await user.click(within(dialog).getByRole("button", { name: /cancel/i }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(backend.getPutCount()).toBe(0);
  });
});

// Which edits take a binding's broker preferences off. Only a move to another
// provider or another address does: a new model at the same OpenRouter address
// is still the binding the preferences were written for.
