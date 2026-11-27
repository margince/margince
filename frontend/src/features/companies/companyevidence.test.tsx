/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { EvidenceModal } from "./companyevidence";

// The receipt is the affordance that makes the prose above it worth reading, so
// the things it must never do are what get tested: hide a gap, print a
// confidence nobody computed, or merge "read" with "confirmed".

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Receipt = components["schemas"]["ClaimEvidence"];

const SITE_READ: Receipt = {
  entity_type: "profile_field",
  entity_id: "p-1",
  source_kind: "site_read",
  produced_by: "site_read:crawler",
  label: "What they offer",
  value: "Load-shifting software",
  excerpt: "We build load-shifting software for industry.",
  identity: { source_url: "https://voltaq.example/about" },
  retrieved_at: "2026-08-01T09:00:00Z",
  confidence: 0.9,
  claim_version: 2,
};

function show(
  receipt: unknown,
  // The stepper the citing card supplies. Absent by default, which is the
  // ordinary case: a card with no ordering to give passes none.
  onStep?: (direction: -1 | 1) => void,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(receipt), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    ),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <EvidenceModal
          companyId="o-1"
          anchor={false}
          canEdit={false}
          cited={{ entityType: "profile_field", entityId: "p-1" }}
          onClose={() => {}}
          onStep={onStep}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("where a cited value came from", () => {
  it("shows the value, its origin, and the span it was read from", async () => {
    show(SITE_READ);

    expect(
      await screen.findByText(/What they offer: Load-shifting software/),
    ).toBeTruthy();
    expect(
      screen.getByText(englishCopy["co.evidence.kind.site_read"]),
    ).toBeTruthy();
    expect(
      screen.getByText(/We build load-shifting software for industry/),
    ).toBeTruthy();
    // A link, so the reader can go and check it rather than retype it.
    expect(
      screen.getByRole("link", { name: "https://voltaq.example/about" }),
    ).toBeTruthy();
  });

  it("names what was not recorded instead of leaving a blank", async () => {
    show({
      ...SITE_READ,
      excerpt: null,
      identity: {},
      gaps: ["source_url", "excerpt"],
    });

    // A claim the reader was told is checkable, with nothing to check it
    // against, is worth saying out loud.
    expect(
      await screen.findByText(/Not recorded: Source, Excerpt/),
    ).toBeTruthy();
  });

  it("prints no confidence for a value a contact entered", async () => {
    show({
      entity_type: "profile_field",
      entity_id: "p-1",
      source_kind: "human",
      produced_by: "human:ada",
      label: "What they offer",
      value: "Load-shifting software",
      identity: { actor: "human:ada" },
      gaps: ["verified_at"],
    });

    expect(
      await screen.findByText(englishCopy["co.evidence.kind.human"]),
    ).toBeTruthy();
    // A contact's assertion carries no model confidence, and a percentage
    // beside it would be a number nobody computed.
    expect(screen.queryByText(/confident/)).toBeNull();
  });

  it("keeps read apart from confirmed", async () => {
    show({
      ...SITE_READ,
      retrieved_at: "2026-08-01T09:00:00Z",
      last_verified_at: "2026-08-06T09:00:00Z",
    });

    // Two different assurances: a machine fetched it, and a contact agreed.
    // Merging them would let a re-read pass for an approval.
    // Anchored on a digit so this does not also match the "Read from their
    // website" origin badge, which is a different claim entirely.
    expect(await screen.findByText(/^Read \d/)).toBeTruthy();
    expect(screen.getByText(/^Confirmed by a person \d/)).toBeTruthy();
  });

  it("will not make a link out of a scheme the browser should not follow", async () => {
    // `source_url` is a plain text column the site-read pipeline fills from
    // crawled pages, and React does not sanitize href. A javascript: value
    // stored there would run on click — a scraped page choosing what our UI
    // executes.
    show({
      ...SITE_READ,
      identity: { source_url: "javascript:alert(1)" },
    });

    expect(await screen.findByText("javascript:alert(1)")).toBeTruthy();
    // Still shown, because the reader opened this to see where the claim came
    // from and hiding an odd source withholds exactly that. Just not clickable.
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("reports a receipt it cannot read as exactly that", async () => {
    show({ entity_type: "profile_field", entity_id: "p-1" });

    expect(await screen.findByText(/Source could not be loaded/)).toBeTruthy();
  });
});

describe("the receipt is a drawer beside the claim, not a box over it", () => {
  // The reader is comparing the receipt against the sentence that cited it.
  // A centred dialog covers that sentence; the drawer leaves it on screen.
  it("opens right-anchored", async () => {
    show(SITE_READ);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.classList.contains("modal-drawer")).toBe(true);
  });

  // Prev/next walks the list the CITING CARD owns. A card that offered no
  // ordering gets no arrows, rather than arrows that step somewhere the
  // reader cannot predict.
  it("offers no stepping when the card gave it no order", async () => {
    show(SITE_READ);
    await screen.findByRole("dialog");
    expect(screen.queryByRole("button", { name: "Next claim" })).toBeNull();
  });

  it("steps forward and back when the card gave it one", async () => {
    const steps: number[] = [];
    show(SITE_READ, (direction) => steps.push(direction));
    await screen.findByRole("dialog");

    await userEvent.click(screen.getByRole("button", { name: "Next claim" }));
    await userEvent.click(
      screen.getByRole("button", { name: englishCopy["co.evidence.previous"] }),
    );
    expect(steps).toEqual([1, -1]);
  });

  // The badge is DERIVED: a machine wrote it and no contact has verified it
  // since. Not stored, so the predicate has to be right here.
  it("marks a machine-written claim nobody has confirmed", async () => {
    show(SITE_READ);
    expect(
      await screen.findByText(englishCopy["co.evidence.extractedUnconfirmed"]),
    ).toBeTruthy();
  });

  it("drops the mark once a contact has verified it", async () => {
    show({ ...SITE_READ, last_verified_at: "2026-08-05T09:00:00Z" });
    await screen.findByRole("dialog");
    expect(screen.queryByText(/AI extracted/)).toBeNull();
  });

  // Only a site read is a model extraction. The other four are not, and the
  // badge would be false of each: a human typed a human value, an older
  // system holds a migration one, a connector value came out of an API
  // verbatim, and a rule value was computed by code somebody wrote.
  it.each(["human", "migration", "connector", "rule"] as const)(
    "never marks a %s claim as AI extracted",
    async (kind) => {
      show({ ...SITE_READ, source_kind: kind, produced_by: "x" });
      await screen.findByRole("dialog");
      expect(screen.queryByText(/AI extracted/)).toBeNull();
    },
  );
});

// The reader who sees a claim is the one who can rule on it. The stub keeps
// the rows' state, so a re-read returns what the server would now return.
// The version a receipt shows is the one a verdict pins.
describe("ruling on the claim from the drawer", () => {
  type Row = {
    id: string;
    field: string;
    value: string;
    source: string;
    captured_by: string;
    updated_at: string;
    version: number;
    verified_at?: string;
  };
  const row = (id: string, value: string, version: number): Row => ({
    id,
    field: id === "p-1" ? "icp" : "usp",
    value,
    source: "site_read",
    captured_by: "agent:deepread",
    updated_at: "2026-08-01T09:00:00Z",
    version,
  });

  function json(body: unknown) {
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }

  // `listVersion` lets a test serve a list that lags the receipt.
  function serve(listVersion?: number) {
    const rows = [
      row("p-1", "Load-shifting software", 2),
      row("p-2", "Grid flexibility", 5),
    ];
    const writes: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = new URL(request.url);
        if (request.method !== "GET") {
          writes.push(
            `${request.method} ${url.pathname} if-match=${request.headers.get("If-Match")}`,
          );
          const target = rows.find((each) => url.pathname.includes(each.field));
          if (target) {
            target.source = "human";
            target.verified_at = "2026-08-06T09:00:00Z";
            target.version += 1;
          }
          return json({});
        }
        if (url.pathname.endsWith("/profile-fields")) {
          return json({
            data: rows.map((each) =>
              listVersion === undefined
                ? each
                : { ...each, version: listVersion },
            ),
          });
        }
        if (url.pathname.endsWith("/facts")) {
          return json({ data: [] });
        }
        const shown = rows.find((each) => url.pathname.endsWith(each.id));
        if (!shown) {
          return json({});
        }
        const confirmed = shown.source === "human";
        return json({
          ...SITE_READ,
          entity_id: shown.id,
          value: shown.value,
          claim_version: shown.version,
          source_kind: confirmed ? "human" : "site_read",
          last_verified_at: shown.verified_at ?? null,
        });
      }),
    );
    return writes;
  }

  function Drawer({ canEdit, anchor }: { canEdit: boolean; anchor?: boolean }) {
    const [id, setId] = useState("p-1");
    return (
      <EvidenceModal
        companyId="o-1"
        anchor={anchor ?? false}
        canEdit={canEdit}
        cited={{ entityType: "profile_field", entityId: id }}
        onClose={() => {}}
        onStep={() => setId(id === "p-1" ? "p-2" : "p-1")}
      />
    );
  }

  function open(canEdit: boolean, client = newClient(), anchor = false) {
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <Drawer canEdit={canEdit} anchor={anchor} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  }

  function newClient() {
    return new QueryClient({ defaultOptions: { queries: { retry: false } } });
  }

  it("confirms from the drawer and the unconfirmed mark is gone on the next read", async () => {
    const writes = serve();
    open(true);
    expect(
      await screen.findByText(englishCopy["co.evidence.extractedUnconfirmed"]),
    ).toBeTruthy();

    await userEvent.click(
      await screen.findByRole("button", { name: "Confirm" }),
    );

    await waitFor(() =>
      expect(
        screen.queryByText(englishCopy["co.evidence.extractedUnconfirmed"]),
      ).toBeNull(),
    );
    expect(writes).toEqual([
      "POST /v1/companies/o-1/profile-fields/icp/confirm if-match=2",
    ]);
    // The snippet the machine read stays beside the human's word.
    expect(
      screen.getByText(/We build load-shifting software for industry/),
    ).toBeTruthy();
  });

  it("offers neither verb to a reader who may not change the company", async () => {
    serve();
    open(false);
    await screen.findByText(englishCopy["co.evidence.extractedUnconfirmed"]);
    expect(screen.queryByRole("button", { name: "Confirm" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Correct" })).toBeNull();
  });

  it("withholds the verbs when the list row is another version than the receipt", async () => {
    serve(9);
    open(true);
    await screen.findByText(englishCopy["co.evidence.extractedUnconfirmed"]);
    // Let the list settle before asserting an absence.
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole("button", { name: "Confirm" })).toBeNull();
  });

  it("does not carry a correction typed for one claim over to the next", async () => {
    const writes = serve();
    open(true);
    await userEvent.click(
      await screen.findByRole("button", { name: "Correct" }),
    );
    const box = await screen.findByRole("textbox", {
      name: englishCopy["evidence.correctedValue"],
    });
    await userEvent.clear(box);
    await userEvent.type(box, "Typed for the first claim");

    await userEvent.click(screen.getByRole("button", { name: "Next claim" }));
    await screen.findByText(/Grid flexibility/);

    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByText("Typed for the first claim")).toBeNull();
    expect(writes).toEqual([]);
  });

  it("refetches the dossier after a correction", async () => {
    serve();
    const client = newClient();
    client.setQueryData(["company-dossier", "o-1"], { sections: [] });
    open(true, client);
    await userEvent.click(
      await screen.findByRole("button", { name: "Correct" }),
    );
    const box = await screen.findByRole("textbox", {
      name: englishCopy["evidence.correctedValue"],
    });
    await userEvent.clear(box);
    await userEvent.type(box, "Better wording");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(
        client.getQueryState(["company-dossier", "o-1"])?.isInvalidated,
      ).toBe(true),
    );
  });
});
