/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import { pickOption, pickSuggestion } from "../design-system/select-testing";
import { AiRoutingCard } from "./ai-routing";
import {
  BOUND,
  backendFor,
  type CapturedRouting,
  openLane,
  ROUTING_EDITOR,
  ROUTING_READER,
  render,
  UNBOUND,
} from "./ai-routing.testkit";
import { rebind } from "./ai-routing-fields";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AiRoutingCard", () => {
  // ── the FIRST binding ──────────────────────────────────────────────────
  //
  // An installation with no tiers used to get a sentence and nothing else: "a
  // deployment declares its first binding under seeds.ai_routing". True of a
  // deployment and false for everyone else — that seed is consumed once, at
  // company creation, so an installation that ALREADY EXISTS can never
  // take one. The desktop bundles ship a database, so their company was
  // created on the build machine, and their recipient reached this screen with
  // no way forward but curl or deleting the demo data they were given.
  it("offers a first binding from a keyed provider when nothing is bound", async () => {
    const backend = backendFor(ROUTING_EDITOR, UNBOUND);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const start = await screen.findByRole("button", {
      name: /start from google gemini/i,
    });
    // Only the vendor that HOLDS a key. Binding a lane at a vendor with no
    // credential fails closed at the first call, which is a state this screen
    // exists to make visible rather than to create.
    expect(
      screen.queryByRole("button", { name: /start from.*anthropic/i }),
    ).toBeNull();

    await userEvent.click(start);

    // The form arrives complete: every tier the contract declares plus the
    // embeddings binding, because a document missing either is refused and the
    // reader would be exactly where they started.
    await userEvent.click(
      await screen.findByRole("button", { name: /preview effects/i }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /save routing/i }),
    );
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut() as CapturedRouting;
    expect(Object.keys(sent.tiers).sort()).toEqual([
      "cheap_cloud",
      "frontier",
      "local_large",
      "local_small",
      "premium",
    ]);
    expect(sent.embeddings.model).toBe("gemini-embedding-001");
    expect(sent.tiers.premium.provider).toBe("gemini");
  });

  // Nothing to bind TO. The seed sentence is still right for a deployment, and
  // the half a reader of THIS screen can act on is "add a key first" — so the
  // callout says that and offers no button that could only fail.
  it("asks for a key first when no provider has one", async () => {
    const backend = backendFor(ROUTING_EDITOR, UNBOUND, {
      providerKeys: [
        { provider: "gemini", configured: false, env_var: "GEMINI_API_KEY" },
        {
          provider: "anthropic",
          configured: false,
          env_var: "ANTHROPIC_API_KEY",
        },
      ],
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    expect(await screen.findByText(/add a provider key/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /start from/i })).toBeNull();
  });

  // A reader who may not change the binding still SEES why it is absent — the
  // same rule the save button follows here: disabled, never hidden.
  it("shows the first-binding offer disabled to a reader who cannot save", async () => {
    const backend = backendFor(ROUTING_READER, UNBOUND);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    expect(
      (
        (await screen.findByRole("button", {
          name: /start from google gemini/i,
        })) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it("shows the bound model for each tier", async () => {
    vi.stubGlobal("fetch", backendFor(ROUTING_EDITOR).fetchMock);
    render(<AiRoutingCard />);

    // Read off the row itself: a lane reports its binding without being
    // opened, which is the whole point of folding the fields away.
    expect(await screen.findByText("gemini-3.5-flash")).toBeTruthy();
    expect(screen.getByText("gemini-3.1-flash-lite")).toBeTruthy();
  });

  // A routing-read-only role sees "AI by activity" stand as a section with a
  // sentence naming the two grants it lacks; the live feature table never
  // renders for it, because the server refuses that data to this grant
  // combination regardless of how the component is shaped. The bindings
  // this grant DOES allow — the lane rows below it — still render, so this
  // is one section's refusal, not a whole-page one.
  it("explains the withheld AI-activity section to a routing-read-only role", async () => {
    vi.stubGlobal("fetch", backendFor(ROUTING_READER).fetchMock);
    render(<AiRoutingCard />);

    expect(await screen.findByText("gemini-3.5-flash")).toBeTruthy();

    expect(screen.getByText("AI by activity")).toBeTruthy();
    expect(
      screen.getByText(
        "Only a user with both AI diagnostics read and AI allowance read can see which features are live now.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("sends the WHOLE binding, so an untouched tier is not dropped", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    const model = within(tier).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-3.1-pro-preview");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    // PUT replaces the whole document, so the tier nobody touched has to travel
    // with the one that changed — sending only the edit would unbind the rest.
    expect(backend.getCapturedPut()).toEqual({
      profile: "eu_hosted",
      tiers: {
        premium: { provider: "gemini", model: "gemini-3.1-pro-preview" },
        cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
      },
      embeddings: { provider: "gemini", model: "gemini-embedding-001" },
    });
  });

  it.each([
    ROUTING_READER,
    { ai_routing: ["read", "update"] } satisfies GrantSpec,
  ])(
    "requires both routing-update and allowance-read before offering changes: %j",
    async (grants) => {
      vi.stubGlobal("fetch", backendFor(grants).fetchMock);
      render(<AiRoutingCard />);

      // Disabled, never hidden: somebody who cannot change the binding still
      // needs to see which vendor their installation's text goes to.
      expect(await screen.findByText("gemini-3.5-flash")).toBeTruthy();
      expect(
        screen.getByRole("button", { name: "Preview effects" }),
      ).toBeDisabled();
      // `disabled`, not `aria-disabled`: the design system reserves the second
      // for a write in flight, so a reader keeps focus on the control they just
      // pressed. A refusal is the first, and the reason travels with it.
      await waitFor(() =>
        expect(
          (
            screen.getByRole("button", {
              name: /save routing/i,
            }) as HTMLButtonElement
          ).disabled,
        ).toBe(true),
      );
    },
  );

  it("says an unbound installation is unbound rather than drawing an empty form", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ROUTING_EDITOR, {
        profile: "eu_hosted",
        tiers: {},
        embeddings: { provider: "", model: "" },
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    expect(await screen.findByText(/no models bound/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /save routing/i })).toBeNull();
  });

  // Cheapest first, most capable last — the ladder, not the alphabet.
  // Alphabetically `cheap_cloud` sits above `frontier` and `local_small` above
  // `premium`, an order that tells a reader nothing about what they are
  // choosing between.
  it("lists the tiers in ladder order, not alphabetically", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ROUTING_EDITOR, {
        profile: "eu_hosted",
        tiers: {
          frontier: { provider: "gemini", model: "f" },
          local_small: { provider: "gemini", model: "ls" },
          premium: { provider: "gemini", model: "p" },
          cheap_cloud: { provider: "gemini", model: "cc" },
        },
        embeddings: { provider: "gemini", model: "e" },
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    await screen.findByText("ls");
    const shown = screen
      .getAllByTestId(/^ai-routing-tier-/)
      .map((el) => el.getAttribute("data-testid"));
    expect(shown).toEqual([
      "ai-routing-tier-local_small",
      "ai-routing-tier-cheap_cloud",
      "ai-routing-tier-premium",
      "ai-routing-tier-frontier",
    ]);
  });

  // The embed lane was missing from this form entirely, so a reader could
  // re-point every chat tier and go on sending their retrieval to the vendor
  // they had just moved away from, with nothing on screen saying so.
  it("lets the embedding lane be re-pointed, and sends it", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-embedding-001");

    const lane = await openLane(user, "ai-routing-embeddings");
    const model = within(lane).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-embedding-002");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.embeddings.model).toBe(
      "gemini-embedding-002",
    );
  });

  // openai_compatible has no default host and the server refuses a binding
  // without one. With no field for it, choosing that adapter produced a write
  // the running role could never adopt: saved cleanly, then declined at the
  // rebind, leaving the OLD models serving with the reason only in a log.
  it("asks for a host when the adapter has no default, and only then", async () => {
    // An instance rather than the default export: pickOption drives a portalled
    // listbox and needs a session that keeps pointer state across the open.
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    // A native vendor addresses its own API, so no host is asked for.
    expect(screen.queryByLabelText("Host")).toBeNull();

    const tier = await openLane(user, "ai-routing-tier-premium");
    // Named, because the row now holds two comboboxes: the adapter, and the
    // model picker that offers what this installation can price.
    await pickOption(
      user,
      within(tier).getByRole("combobox", { name: "Provider" }),
      "openai_compatible",
    );
    const host = await within(tier).findByLabelText("Host");
    await user.type(host, "https://openrouter.ai/api");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut();
    expect(sent?.tiers.premium.provider).toBe("openai_compatible");
    expect(sent?.tiers.premium.base_url).toBe("https://openrouter.ai/api");
  });
  // Broker preferences belong to the OpenRouter binding they were written for,
  // and the server refuses them on any other. Carried onto a new vendor, they
  // would turn a routine switch into a save refused with no field to fix.
  it("drops a tier's broker preferences when it leaves OpenRouter", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, {
      ...BOUND,
      tiers: {
        ...BOUND.tiers,
        premium: {
          provider: "openai_compatible",
          model: "openai/gpt-oss-120b",
          base_url: "https://openrouter.ai/api",
          routing: { sort: "throughput" },
        },
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("openai/gpt-oss-120b");

    const tier = await openLane(user, "ai-routing-tier-premium");
    await pickOption(
      user,
      within(tier).getByRole("combobox", { name: "Provider" }),
      "gemini",
    );
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut()?.tiers.premium;
    expect(sent?.provider).toBe("gemini");
    expect(sent).not.toHaveProperty("routing");
  });
  // The lane the operator reported as unreachable: it takes a provider of its
  // own, and re-pointing it has to carry the host and the width with it or the
  // server refuses the binding it just accepted.
  it("re-points the embedding lane onto a hosted adapter, with its width", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-embedding-001");

    const lane = await openLane(user, "ai-routing-embeddings");
    await pickOption(
      user,
      within(lane).getByRole("combobox", { name: "Provider" }),
      "openai_compatible",
    );
    await user.type(
      await within(lane).findByLabelText("Host"),
      "https://openrouter.ai/api",
    );
    await user.type(within(lane).getByLabelText("Vector width"), "1536");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut()?.embeddings;
    expect(sent?.provider).toBe("openai_compatible");
    expect(sent?.base_url).toBe("https://openrouter.ai/api");
    expect(sent?.dimensions).toBe(1536);
  });

  // An emptied width means "whatever the provider compiles in", which the
  // contract spells as an absent field. A 0 is a different instruction, and a
  // NaN does not survive the JSON at all, so neither may reach the wire.
  it("sends no width at all when the field is emptied, rather than a zero", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-embedding-001");

    const lane = await openLane(user, "ai-routing-embeddings");
    const width = within(lane).getByLabelText("Vector width");
    await user.type(width, "768");
    await user.clear(width);
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut()?.embeddings;
    expect(sent?.dimensions).toBeUndefined();
    expect(JSON.stringify(sent)).not.toContain("dimensions");
  });

  // The models this installation can PRICE, offered per lane. A tier picker
  // that listed the embedder would offer a model that cannot serve one call,
  // and an embed picker that listed the chat models would do the same.
  it("offers the priced models for the bound provider, in that row's lane", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    await user.click(within(tier).getByRole("combobox", { name: "Model" }));
    // The row says both: which model, and what it costs per million tokens in
    // → out, which is what a reader is choosing between.
    //
    // The VENDOR's models first, in the order it returned them, then whatever
    // the sheet prices that the vendor did not name — sorted, so the tail is
    // stable. A price appears where the sheet has one and nowhere else.
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual([
      "gemini-4.0-flash",
      "gemini-3.5-flashInput US$1.50 · Output US$9.00 per 1M tokens",
      "gemini-3.1-flash-liteInput US$0.25 · Output US$1.50 per 1M tokens",
      "gemini-3.1-pro-previewInput US$2.00 · Output US$12.00 per 1M tokens",
    ]);
    // Neither the embedder nor another vendor's model.
    expect(offered.join(" ")).not.toContain("gemini-embedding-001");
    expect(offered.join(" ")).not.toContain("claude-opus-4-8");
  });

  it("binds the model a reader picks off the list", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    await pickSuggestion(
      user,
      within(tier).getByRole("combobox", { name: "Model" }),
      /^gemini-3\.1-pro-preview/,
    );
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.premium.model).toBe(
      "gemini-3.1-pro-preview",
    );
  });

  // The defect this replaced: the picker offered the price sheet alone, so a
  // model the vendor shipped after somebody last edited that table was absent
  // and a reader concluded the product could not reach it.
  it("offers what the VENDOR serves, including a model the sheet never priced", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(ROUTING_EDITOR).fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    await user.click(within(tier).getByRole("combobox", { name: "Model" }));
    const offered = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);

    // Newest first, in the vendor's own order, and priced only where the sheet
    // can price it — the new model carries no figure rather than a zero.
    expect(offered[0]).toBe("gemini-4.0-flash");
    expect(offered[1]).toBe(
      "gemini-3.5-flashInput US$1.50 · Output US$9.00 per 1M tokens",
    );
    // The embedder the vendor DID declare stays off a chat lane: it cannot
    // serve one, and offering it would bind a call that must fail.
    expect(offered.join(" ")).not.toContain("gemini-embedding-001");
  });

  // A vendor that cannot be asked degrades to the sheet and says which state it
  // is in, rather than emptying the field or failing the form.
  it("falls back to the sheet, and says why, when the vendor holds no key", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(ROUTING_EDITOR).fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    await pickOption(
      user,
      within(tier).getByRole("combobox", { name: "Provider" }),
      "anthropic",
    );
    expect(
      await within(tier).findByText(/this provider has no key/i),
    ).toBeTruthy();
    // And the sheet's own anthropic row is still offered, so the field is not
    // left empty by a vendor it could not reach. The box is cleared first: it
    // still holds the previous vendor's model, and the list filters on what is
    // typed.
    const model = within(tier).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.click(model);
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((o) => o.textContent)
        .join(" "),
    ).toContain("claude-opus-4-8");
  });

  // The half that keeps this from being a Select: the sheet is a starting
  // point, the server takes any id its vendor serves, and a vendor ships a
  // model on a Tuesday.
  it("binds a model the sheet has never heard of", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    const model = within(tier).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-4-experimental-0731");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.premium.model).toBe(
      "gemini-4-experimental-0731",
    );
  });

  // A seat holding the routing grant but not the sheet's own, or an
  // installation whose sheet was never seeded.
  //
  // This used to leave the field with no suggestions at all, because the sheet
  // was the only source. The VENDOR is now the other one and it is still
  // answering, so an unreadable sheet costs the reader the PRICES beside each
  // id rather than the list itself — and the form still saves either way.
  it("still offers the vendor's models when the sheet cannot be read", async () => {
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR, BOUND, { sheetStatus: 403 });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByText("gemini-3.5-flash");

    const tier = await openLane(user, "ai-routing-tier-premium");
    const model = within(tier).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.click(model);
    expect(
      within(await screen.findByRole("listbox"))
        .getAllByRole("option")
        .map((o) => o.textContent),
      // Every id the vendor named, and not one price: the sheet is what
      // carries those and it is not this reader's to read.
    ).toEqual(["gemini-4.0-flash", "gemini-3.5-flash"]);

    await user.type(model, "gemini-3.1-pro-preview");
    await user.click(screen.getByRole("button", { name: /preview effects/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save routing/i }),
      ).not.toBeDisabled(),
    );
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.premium.model).toBe(
      "gemini-3.1-pro-preview",
    );
  });
});

describe("rebind", () => {
  const openRouterTier = {
    provider: "openai_compatible",
    model: "openai/gpt-oss-120b",
    base_url: "https://openrouter.ai/api",
    routing: { sort: "throughput" },
  };

  it("drops them when the model changes, since a pin names one model's hosts", () => {
    const next = rebind(openRouterTier, {
      model: "mistralai/mistral-large-2512",
    });
    expect(next.model).toBe("mistralai/mistral-large-2512");
    expect(next).not.toHaveProperty("routing");
  });

  it("drops them when the provider changes, so the save is not refused", () => {
    const next = rebind(openRouterTier, { provider: "gemini" });
    expect(next.provider).toBe("gemini");
    expect(next).not.toHaveProperty("routing");
  });

  it("drops them when base_url moves off OpenRouter", () => {
    const next = rebind(openRouterTier, { base_url: "https://api.mistral.ai" });
    expect(next.base_url).toBe("https://api.mistral.ai");
    expect(next).not.toHaveProperty("routing");
  });

  it("keeps them when the patch re-states the same provider and base_url", () => {
    const next = rebind(openRouterTier, {
      provider: "openai_compatible",
      base_url: "https://openrouter.ai/api",
    });
    expect(next.routing).toEqual({ sort: "throughput" });
  });
});
