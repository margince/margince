/** @vitest-environment happy-dom */
import type { ExtensionDescriptor } from "@composition/extensions";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import type {
  ExtensionPanelProps,
  ExtensionPanelRegistry,
} from "../lib/extensions";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { ExtensionPanels } from "./extensionpanels";

// The vanilla registry is empty, so each case hands the slot the descriptors
// and components the generator would compose.
function unit(name: string, object: string): ExtensionDescriptor {
  return {
    name,
    title: "",
    verbs: [],
    entries: [],
    panels: [{ slot: "record_panel.deal", export: "DealPanel", object }],
    secretScope: "",
  };
}

function Says({ record }: ExtensionPanelProps) {
  return <p>channel for {record.id}</p>;
}

function Tally({ record }: ExtensionPanelProps) {
  return <p>tally for {record.id}</p>;
}

function renderSlot(
  allow: GrantSpec,
  registry: readonly ExtensionDescriptor[],
  components: ExtensionPanelRegistry,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ allow }));
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <ExtensionPanels
          slot="record_panel.deal"
          record={{ type: "deal", id: "d-1" }}
          registry={registry}
          components={components}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(cleanup);

describe("ExtensionPanels", () => {
  it("draws nothing when no composed unit fills the slot", () => {
    const { container } = renderSlot({}, [], {});
    expect(container.innerHTML).toBe("");
  });

  it("draws each unit's panel, in registry order, with the page's record", () => {
    renderSlot(
      { ext_alpha_x: ["read"], ext_beta_x: ["read"] },
      [unit("alpha", "ext_alpha_x"), unit("beta", "ext_beta_x")],
      {
        alpha: { "record_panel.deal": Says },
        beta: { "record_panel.deal": Tally },
      },
    );
    const texts = screen.getAllByText(/for d-1/).map((p) => p.textContent);
    expect(texts).toEqual(["channel for d-1", "tally for d-1"]);
  });

  it.each([
    ["record_panel.contact", "contact"],
    ["record_panel.company", "company"],
  ] as const)(
    "draws a unit's %s panel with the record's type and id",
    (slot, type) => {
      const client = new QueryClient();
      client.setQueryData(
        ["me"],
        meFixture({ allow: { ext_alpha_x: ["read"] } }),
      );
      const withSlot: ExtensionDescriptor = {
        ...unit("alpha", "ext_alpha_x"),
        panels: [{ slot, export: "DealPanel", object: "ext_alpha_x" }],
      };
      render(
        <QueryClientProvider client={client}>
          <LocaleProvider>
            <ExtensionPanels
              slot={slot}
              record={{ type, id: "r-1" }}
              registry={[withSlot]}
              components={{ alpha: { [slot]: Says } }}
            />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      expect(screen.getByText("channel for r-1")).toBeTruthy();
    },
  );

  it("groups the panels under one Extensions label, apart from core's cards", () => {
    renderSlot({ ext_alpha_x: ["read"] }, [unit("alpha", "ext_alpha_x")], {
      alpha: { "record_panel.deal": Says },
    });
    const group = screen.getByRole("region", { name: "Extensions" });
    expect(group.textContent).toContain("channel for d-1");
  });

  it("hides a panel from a reader without read on its object", () => {
    renderSlot(
      { ext_beta_x: ["read"] },
      [unit("alpha", "ext_alpha_x"), unit("beta", "ext_beta_x")],
      {
        alpha: { "record_panel.deal": Says },
        beta: { "record_panel.deal": Tally },
      },
    );
    expect(screen.queryByText("channel for d-1")).toBeNull();
    expect(screen.getByText("tally for d-1")).toBeTruthy();
  });

  it("draws nothing for a declared panel the registry has no component for", () => {
    const { container } = renderSlot(
      { ext_alpha_x: ["read"] },
      [unit("alpha", "ext_alpha_x")],
      {},
    );
    expect(container.innerHTML).toBe("");
  });

  // A unit named after a prototype key reads no component off the chain.
  it("draws nothing for a unit named constructor with no component", () => {
    const { container } = renderSlot(
      { ext_constructor_x: ["read"] },
      [unit("constructor", "ext_constructor_x")],
      {},
    );
    expect(container.innerHTML).toBe("");
  });

  it("starts a failed panel afresh on the next deal", () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    function OnlyD2({ record }: ExtensionPanelProps) {
      if (record.id !== "d-2") {
        throw new Error("unit bug");
      }
      return <p>fine for {record.id}</p>;
    }
    const client = new QueryClient();
    client.setQueryData(
      ["me"],
      meFixture({ allow: { ext_alpha_x: ["read"] } }),
    );
    const slot = (id: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <ExtensionPanels
            slot="record_panel.deal"
            record={{ type: "deal", id }}
            registry={[unit("alpha", "ext_alpha_x")]}
            components={{ alpha: { "record_panel.deal": OnlyD2 } }}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(slot("d-1"));
    expect(screen.getByRole("alert")).toBeTruthy();
    rerender(slot("d-2"));
    expect(screen.getByText("fine for d-2")).toBeTruthy();
    quiet.mockRestore();
  });

  it("keeps one unit's crash in its own card, and retries it", async () => {
    // React logs the caught throw itself; silenced so the run stays readable.
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    let broken = true;
    function Flaky({ record }: ExtensionPanelProps) {
      if (broken) {
        throw new Error("unit bug");
      }
      return <p>recovered for {record.id}</p>;
    }
    renderSlot(
      { ext_alpha_x: ["read"], ext_beta_x: ["read"] },
      [unit("alpha", "ext_alpha_x"), unit("beta", "ext_beta_x")],
      {
        alpha: { "record_panel.deal": Flaky },
        beta: { "record_panel.deal": Tally },
      },
    );
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("tally for d-1")).toBeTruthy();
    broken = false;
    await userEvent.setup().click(screen.getByRole("button"));
    expect(await screen.findByText("recovered for d-1")).toBeTruthy();
    quiet.mockRestore();
  });
});
