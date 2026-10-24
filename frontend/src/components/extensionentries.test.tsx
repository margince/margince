/** @vitest-environment happy-dom */
import type { ExtensionDescriptor } from "@composition/extensions";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { ExtensionEntries } from "./extensionentries";

// The vanilla registry is empty, so each case hands the component the shape
// the generator emits. The label is a core key, because the merged catalogue
// holds no unit copy in this lane and a core key renders the same way.
function unit(
  name: string,
  entries: ExtensionDescriptor["entries"],
): ExtensionDescriptor {
  return { name, title: "", verbs: [], entries, secretScope: "" };
}

const VISITS = unit("visits", [
  {
    slot: "record_header.contact",
    label: "extUnits.user.title",
    icon: "map-pin",
    target: "report",
    object: "ext_visits_report",
  },
  {
    slot: "log_activity",
    label: "extUnits.workspace.title",
    icon: "clipboard-list",
    target: "",
    object: "ext_visits_report",
  },
]);

function renderEntries(
  allow: GrantSpec,
  ui: Parameters<typeof ExtensionEntries>[0],
) {
  const client = new QueryClient();
  client.setQueryData(["me"], meFixture({ allow }));
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <ExtensionEntries {...ui} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  globalThis.location.hash = "#/contacts/c-1";
});

afterEach(cleanup);

describe("ExtensionEntries", () => {
  it("opens the unit's view with the record it was pressed on", async () => {
    const user = userEvent.setup();
    renderEntries(
      { ext_visits_report: ["read"] },
      {
        slot: "record_header.contact",
        record: { type: "contact", id: "c-1" },
        registry: [VISITS],
      },
    );
    await user.click(screen.getByTestId("ext-entry-visits"));
    expect(globalThis.location.hash).toBe(
      "#/ext/visits/report?record_id=c-1&record_type=contact",
    );
  });

  it("shows only the entries declared for its own slot", () => {
    renderEntries(
      { ext_visits_report: ["read"] },
      { slot: "record_header.deal", registry: [VISITS] },
    );
    expect(screen.queryByTestId("ext-entry-visits")).toBeNull();
  });

  it("hides an entry from a reader without read on its object", () => {
    renderEntries(
      { ext_visits_report: ["create"] },
      { slot: "log_activity", registry: [VISITS] },
    );
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("opens the unit's screen itself when the entry names no view", async () => {
    const user = userEvent.setup();
    renderEntries(
      { ext_visits_report: ["read"] },
      { slot: "log_activity", inRow: true, registry: [VISITS] },
    );
    await user.click(screen.getByTestId("ext-entry-visits"));
    expect(globalThis.location.hash).toBe("#/ext/visits");
  });

  describe("a mark slot", () => {
    const MARKED = unit("visits", [
      {
        slot: "record_mark.contact",
        label: "extUnits.user.title",
        icon: "map-pin",
        target: "",
        object: "ext_visits_report",
      },
    ]);

    it("draws the note for a reader holding read on its object", () => {
      renderEntries(
        { ext_visits_report: ["read"] },
        { slot: "record_mark.contact", registry: [MARKED] },
      );
      expect(screen.getByText(/\S/)).toBeTruthy();
      expect(screen.queryByRole("button")).toBeNull();
    });

    it("draws no note for a reader without read on its object", () => {
      const { container } = renderEntries(
        { ext_visits_report: ["create"] },
        { slot: "record_mark.contact", registry: [MARKED] },
      );
      expect(container.innerHTML).toBe("");
    });
  });

  it("draws nothing once the unit is no longer composed", () => {
    const { container } = renderEntries(
      { ext_visits_report: ["read"] },
      { slot: "log_activity", inRow: true, registry: [] },
    );
    expect(container.innerHTML).toBe("");
  });
});
