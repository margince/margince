/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import {
  EvidenceVerdict,
  factClaim,
  profileFieldClaim,
} from "./evidenceverdict";

// ADR-0085 gave a human two verbs over a machine's claim, and for a while
// nothing in the product could call either. These pin that the buttons reach
// the right endpoint with the right body — a confirm that quietly PATCHed, or a
// correction addressed by the wrong key, looks identical on screen.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const COMPANY = "o-1";

// The version is what both verbs pin. A fixture without it models a row the
// server does not produce, and every read that offers a verdict now returns
// one — so a test that omitted it would prove the buttons work against a
// record shaped unlike the real one.
const field: components["schemas"]["CompanyProfileField"] = {
  field: "industry",
  value: "Fahrzeugbau",
  source: "site_read",
  captured_by: "agent:deepread",
  updated_at: "2026-08-01T09:00:00Z",
  version: 3,
};

// A different number from the profile field's, so a verb reaching for the wrong
// row's version fails here rather than passing on a coincidence.
const fact: components["schemas"]["CompanyFact"] = {
  category: "company",
  field: "phone",
  value: "+49 30 1234",
  value_key: "",
  source: "site_read",
  captured_by: "agent:deepread",
  id: "f-1",
  updated_at: "2026-08-01T09:00:00Z",
  version: 7,
};

// The calls the component made, so a test can assert the METHOD and the PATH
// rather than that something happened.
function recordCalls() {
  const calls: {
    method: string;
    path: string;
    body?: unknown;
    ifMatch: string | null;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push({
        method: request.method,
        path: new URL(request.url).pathname,
        body: request.body ? await request.clone().json() : undefined,
        // Both verbs overwrite a row somebody else may have moved, so the
        // precondition is part of what each call IS — asserted here rather than
        // left to a reviewer to notice missing.
        ifMatch: request.headers.get("If-Match"),
      });
      return new Response("{}", {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  return calls;
}

// A server that refuses, answering the RFC-7807 body the contract declares.
function refuseWith(status: number, problem: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(problem), {
          status,
          headers: { "content-type": "application/problem+json" },
        }),
    ),
  );
}

function wrap(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a human's verdict on a machine's claim", () => {
  it("confirms a profile field without changing its value", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={profileFieldClaim(COMPANY, field)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    // POST to the confirm sub-path, with no body: a confirmation that carried a
    // value would be a correction wearing the wrong verb.
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].method).toBe("POST");
    expect(calls[0].path).toBe(
      "/v1/companies/o-1/profile-fields/industry/confirm",
    );
    expect(calls[0].body).toBeUndefined();
    // A confirmation is a contact agreeing with a value they READ. Unpinned, it
    // stamps their name on whatever the row says by the time it lands — which
    // may be a correction they never saw.
    expect(calls[0].ifMatch).toBe("3");
  });

  it("corrects a profile field by sending the new value", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={profileFieldClaim(COMPANY, field)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Correct" }));
    const input = screen.getByRole("textbox", { name: "Corrected value" });
    await user.clear(input);
    await user.type(input, "Automotive");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].method).toBe("PATCH");
    expect(calls[0].path).toBe("/v1/companies/o-1/profile-fields/industry");
    expect(calls[0].body).toEqual({ value: "Automotive" });
    expect(calls[0].ifMatch).toBe("3");
  });

  it("addresses a single-value fact by its bare-colon key", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, fact)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    // `phone:` — the field, the separator, and an empty value_key. A key that
    // lost either half addresses a different row or none at all, and the server
    // answers 404 or 422 for what looks to the user like a working button.
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].path).toBe("/v1/companies/o-1/facts/phone%3A/confirm");
  });

  // The three verbs on a fact row have to agree about whether a precondition
  // matters. Removal has always pinned; a confirm and a correction that did not
  // meant two people ruling on one claim overwrote each other with no conflict
  // and no trace — the failure the version column exists to prevent.
  it("confirms a fact against the row it agrees with", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, fact)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].ifMatch).toBe("7");
  });

  it("corrects a fact against the row it replaces", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, fact)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Correct" }));
    const input = screen.getByRole("textbox", { name: "Corrected value" });
    await user.clear(input);
    await user.type(input, "+49 30 9999");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].method).toBe("PATCH");
    expect(calls[0].path).toBe("/v1/companies/o-1/facts/phone%3A");
    expect(calls[0].body).toEqual({ value: "+49 30 9999" });
    expect(calls[0].ifMatch).toBe("7");
  });

  // Holding no version is holding no claim about what the write overwrites, so
  // the honest answer is to send nothing. An unpinned request would land on top
  // of an edit it never saw and report success to both editors.
  it("sends no verdict on a fact that came back without a version", async () => {
    const user = userEvent.setup();
    const calls = recordCalls();
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, { ...fact, version: undefined })}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(calls).toEqual([]);
  });

  // What the precondition buys: the loser of a race is told, in their own
  // language, instead of being shown a success over a value they never read.
  it("tells the reader the row moved under them", async () => {
    const user = userEvent.setup();
    refuseWith(409, {
      type: "https://errors.gradion.com/version_skew",
      title: "Version conflict",
      status: 409,
      code: "version_skew",
      detail: "The resource changed since your last read; re-read and retry.",
    });
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, fact)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "The resource changed since your last read",
      ),
    );
  });

  // A refusal the server states only as a code carries no sentence for a
  // reader, and the raw Error message is the DEVELOPER's placeholder. The
  // catalog is where a reader's words live, so the translator has to reach the
  // refusal — the same path every other screen's error region takes.
  it("answers a refusal stated only as a code with catalog copy", async () => {
    const user = userEvent.setup();
    refuseWith(403, {
      type: "https://errors.gradion.com/permission_denied",
      status: 403,
      code: "permission_denied",
    });
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={factClaim(COMPANY, fact)}
        canEdit
      />,
    );

    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alert").textContent).toBe(
      "You do not have permission for this action. Ask an admin, or whoever shared this record with you, to widen your access.",
    );
  });

  it("offers no verdict to a reader who cannot update the company", () => {
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={profileFieldClaim(COMPANY, field)}
        canEdit={false}
      />,
    );
    expect(screen.queryByRole("button", { name: "Confirm" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Correct" })).toBeNull();
  });

  it("says who stood behind a value rather than offering to confirm it again", () => {
    wrap(
      <EvidenceVerdict
        companyId={COMPANY}
        claim={profileFieldClaim(COMPANY, {
          ...field,
          source: "human",
          verified_at: "2026-08-02T09:00:00Z",
        })}
        canEdit
      />,
    );
    // A value a contact already stood behind is not a claim awaiting a verdict.
    expect(screen.queryByRole("button", { name: "Confirm" })).toBeNull();
    expect(screen.getByText(/Confirmed by a person/)).toBeTruthy();
  });
});
