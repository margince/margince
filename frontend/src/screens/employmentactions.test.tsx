/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, type Translator, translate } from "../i18n";
import { isVersionSkewOf, ProblemError } from "./common";
import {
  patchEmployment,
  searchCompanyCandidates,
  useEmploymentActions,
} from "./employmentactions";

// The writes behind a contact's employment list, against the wire. The fetch
// stub is the one boundary faked here: what each write sends, and what it does
// with a refusal, is the whole contract of this module.

type Employment = components["schemas"]["Contact360Employment"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const t: Translator = (key, params) => translate("en", key, params);

/** One request as the stub saw it: verb, path, the precondition and the body. */
type Sent = {
  method: string;
  path: string;
  ifMatch: string | null;
  body: unknown;
};

// Answers by "METHOD /path"; records every request so a test can assert on what
// left, not only on what came back.
function wire(routes: Record<string, () => Response>): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const url = new URL(request.url);
      const text = await request.text();
      sent.push({
        method: request.method,
        path: url.pathname,
        ifMatch: request.headers.get("If-Match"),
        body: text === "" ? null : JSON.parse(text),
      });
      const handler = routes[`${request.method} ${url.pathname}`];
      if (!handler) {
        throw new Error(`no route for ${request.method} ${url.pathname}`);
      }
      return handler();
    }),
  );
  return sent;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const employment: Employment = {
  relationship_id: "edge-1",
  company_id: "company-1",
  company_name: "Brandt Automotive GmbH",
  role: "Engineer",
  employment_status: "current",
  is_current_primary: true,
  version: 3,
};

// The list endpoint's row for the same edge, as a re-read returns it.
const fresh = {
  id: "edge-1",
  contact_id: "contact-1",
  company_id: "company-1",
  kind: "employment",
  role: "Engineer",
  employment_status: "current",
  version: 7,
};

function problemOf(error: unknown): unknown {
  if (!(error instanceof ProblemError)) {
    throw new Error("the refusal did not arrive as a problem");
  }
  return error.problem;
}

describe("the company search the add flow picks from", () => {
  it("names each company by id and display name", async () => {
    const sent = wire({
      "GET /v1/companies": () =>
        json({
          data: [
            { id: "company-1", display_name: "Brandt Automotive GmbH" },
            { id: "company-2", display_name: "Sommer Logistik" },
          ],
          page: { has_more: false },
        }),
    });
    await expect(searchCompanyCandidates("bran")).resolves.toEqual([
      { id: "company-1", name: "Brandt Automotive GmbH" },
      { id: "company-2", name: "Sommer Logistik" },
    ]);
    expect(sent[0]?.path).toBe("/v1/companies");
  });

  it("surfaces a refused search as the problem the server sent", async () => {
    wire({
      "GET /v1/companies": () =>
        json({ code: "permission_denied", detail: "Not yours" }, 403),
    });
    await expect(searchCompanyCandidates("bran")).rejects.toThrow(ProblemError);
  });
});

describe("editing one employment edge", () => {
  it("pins the snapshot's own version on the write", async () => {
    const sent = wire({
      "PATCH /v1/relationships/edge-1": () =>
        json({ ...fresh, role: "Lead Engineer", version: 4 }),
    });
    await patchEmployment(
      employment,
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    expect(sent).toEqual([
      {
        method: "PATCH",
        path: "/v1/relationships/edge-1",
        ifMatch: "3",
        body: { role: "Lead Engineer" },
      },
    ]);
  });

  // A snapshot without a version cannot be pinned as it stands. The edge is
  // re-read through the scoped list, and only a row that still shows what the
  // reader saw lends its version — otherwise the write would silently sit on
  // top of a change nobody looked at.
  it("borrows a fresh version for an unversioned snapshot that still matches", async () => {
    const sent = wire({
      "GET /v1/relationships": () =>
        json({ data: [fresh], page: { has_more: false } }),
      "PATCH /v1/relationships/edge-1": () =>
        json({ ...fresh, role: "Lead Engineer", version: 8 }),
    });
    await patchEmployment(
      { ...employment, version: undefined },
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    expect(sent.map((request) => request.method)).toEqual(["GET", "PATCH"]);
    expect(sent[1]?.ifMatch).toBe("7");
  });

  it("refuses an unversioned snapshot whose visible fields have moved on, and writes nothing", async () => {
    const sent = wire({
      "GET /v1/relationships": () =>
        json({
          data: [{ ...fresh, role: "Staff Engineer" }],
          page: { has_more: false },
        }),
    });
    const refusal = patchEmployment(
      { ...employment, version: undefined },
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    await expect(refusal).rejects.toSatisfy(isVersionSkewOf);
    expect(sent.map((request) => request.method)).toEqual(["GET"]);
  });

  it("says so when the re-read no longer finds the edge", async () => {
    wire({
      "GET /v1/relationships": () =>
        json({ data: [], page: { has_more: false } }),
    });
    const refusal = patchEmployment(
      { ...employment, version: undefined },
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    await expect(refusal).rejects.toThrow(
      t("contact.rail.employmentVersionUnresolved"),
    );
  });

  it("surfaces a refused re-read as the problem the server sent", async () => {
    wire({
      "GET /v1/relationships": () =>
        json({ code: "permission_denied", detail: "Not yours" }, 403),
    });
    const refusal = patchEmployment(
      { ...employment, version: undefined },
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    await expect(refusal.catch(problemOf)).resolves.toMatchObject({
      code: "permission_denied",
    });
  });

  it("surfaces a refused write as the problem the server sent", async () => {
    wire({
      "PATCH /v1/relationships/edge-1": () =>
        json({ code: "permission_denied", detail: "Not yours" }, 403),
    });
    const refusal = patchEmployment(
      employment,
      "contact-1",
      { role: "Lead Engineer" },
      t,
    );
    await expect(refusal.catch(problemOf)).resolves.toMatchObject({
      code: "permission_denied",
    });
  });
});

describe("the list's verbs", () => {
  function actions() {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const invalidated: unknown[] = [];
    const original = client.invalidateQueries.bind(client);
    client.invalidateQueries = async (filters) => {
      invalidated.push(filters?.queryKey);
      await original(filters);
    };
    const { result } = renderHook(() => useEmploymentActions("contact-1"), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">{children}</LocaleProvider>
        </QueryClientProvider>
      ),
    });
    return { result, invalidated };
  }

  // Every verb refreshes the same three readings, because the record's
  // employer line, its paged roles and its brief each summarise this list and
  // a saved edit that left one of them stale would show two answers at once.
  const refreshed = [
    ["contact360", "contact-1"],
    ["contactEmployments", "contact-1"],
    ["contactBrief", "contact-1"],
  ];

  it("creates an edge and refreshes the record, its roles and its brief", async () => {
    const sent = wire({
      "POST /v1/relationships": () => json({ ...fresh, id: "edge-2" }),
    });
    const { result, invalidated } = actions();
    await result.current.create.mutateAsync({
      contact_id: "contact-1",
      company_id: "company-2",
      kind: "employment",
      source: "manual",
    });
    expect(sent[0]).toMatchObject({
      method: "POST",
      path: "/v1/relationships",
      body: { company_id: "company-2" },
    });
    await waitFor(() => expect(invalidated).toEqual(refreshed));
  });

  it("ends an edge today, through the same pinned write an edit takes", async () => {
    const sent = wire({
      "PATCH /v1/relationships/edge-1": () =>
        json({ ...fresh, employment_status: "former", version: 4 }),
    });
    const { result } = actions();
    await result.current.end.mutateAsync(employment);
    expect(sent[0]).toMatchObject({
      method: "PATCH",
      path: "/v1/relationships/edge-1",
      ifMatch: "3",
    });
    expect(sent[0]?.body).toMatchObject({
      ended_at: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/),
    });
  });

  it("edits an edge with the body the row supplied", async () => {
    const sent = wire({
      "PATCH /v1/relationships/edge-1": () =>
        json({ ...fresh, role: "Lead Engineer", version: 4 }),
    });
    const { result } = actions();
    await result.current.update.mutateAsync({
      employment,
      body: { role: "Lead Engineer" },
    });
    expect(sent[0]?.body).toEqual({ role: "Lead Engineer" });
  });

  it("removes an edge, and keeps a refusal for the row to show", async () => {
    const sent = wire({
      "DELETE /v1/relationships/edge-1": () =>
        new Response(null, { status: 204 }),
      "DELETE /v1/relationships/edge-9": () =>
        json({ code: "permission_denied", detail: "Not yours" }, 403),
    });
    const { result, invalidated } = actions();
    await result.current.remove.mutateAsync("edge-1");
    expect(sent[0]).toMatchObject({
      method: "DELETE",
      path: "/v1/relationships/edge-1",
    });
    await waitFor(() => expect(invalidated).toEqual(refreshed));

    const refusal = result.current.remove.mutateAsync("edge-9");
    await expect(refusal.catch(problemOf)).resolves.toMatchObject({
      code: "permission_denied",
    });
  });

  it("surfaces a refused create as the problem the server sent", async () => {
    wire({
      "POST /v1/relationships": () =>
        json({ code: "duplicate", detail: "Already connected" }, 409),
    });
    const { result } = actions();
    const refusal = result.current.create.mutateAsync({
      contact_id: "contact-1",
      company_id: "company-1",
      kind: "employment",
      source: "manual",
    });
    await expect(refusal.catch(problemOf)).resolves.toMatchObject({
      code: "duplicate",
    });
  });
});
