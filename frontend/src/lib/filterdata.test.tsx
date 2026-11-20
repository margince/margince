/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en, type MessageKey } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import {
  fieldLabel,
  groupFields,
  PREVIEW_PAGE,
  useFilterPreview,
  useFilterVocabulary,
  type VocabularyField,
} from "./filterdata";
import { newGroup, newLeaf } from "./segmentpredicate";

// These hooks exist to decide WHEN NOT TO ASK, so that is what the tests watch:
// which URLs were requested, and how many times. Asserting on rendered output
// would prove the query resolved and say nothing about the request it did or did
// not make — which is the whole behaviour.

afterEach(cleanup);

const VOCAB_BODY = {
  resource: "contact",
  fields: [
    {
      name: "full_name",
      type: "text",
      operators: ["eq", "contains"],
      custom: false,
    },
  ],
};

function harness() {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      seen.push(String(input instanceof Request ? input.url : input));
      return new Response(
        JSON.stringify({
          ...VOCAB_BODY,
          match_count: 7,
          columns: ["id"],
          rows: [],
          truncated: false,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ rowScope: "all" }));
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { seen, wrapper };
}

describe("the vocabulary read", () => {
  it("asks for the resource it was given", async () => {
    const { seen, wrapper } = harness();
    const { result } = renderHook(() => useFilterVocabulary("company"), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(seen.some((url) => url.includes("resource=company"))).toBe(true);
  });

  it("serves a second reader of the same resource from cache", async () => {
    const { seen, wrapper } = harness();
    const { result } = renderHook(
      () => {
        // Two hooks, one resource — a builder and a picker on the same screen.
        useFilterVocabulary("contact");
        return useFilterVocabulary("contact");
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(
      seen.filter((url) => url.includes("/filters/vocabulary")),
    ).toHaveLength(1);
  });

  it("asks nothing while a surface that only sometimes needs it says not now", async () => {
    const { seen, wrapper } = harness();
    const { result } = renderHook(
      () => ({
        // The enabled read beside it proves the harness answers; the disabled
        // one must still not have asked by the time that answer is in.
        unwanted: useFilterVocabulary("company", false),
        wanted: useFilterPreview(
          "contact",
          newGroup("and", [newLeaf("full_name", "contains", "ann")]),
        ),
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.wanted.isSuccess).toBe(true));
    expect(result.current.unwanted.status).toBe("pending");
    expect(
      seen.filter(
        (url) =>
          url.includes("/filters/vocabulary") || url.includes("/custom-fields"),
      ),
    ).toEqual([]);
  });
});

describe("naming a custom field from its catalog", () => {
  const TIER = {
    name: "cf_tier",
    type: "picklist",
    operators: ["eq"],
    custom: true,
  };

  // The vocabulary and the catalog as the server answers them; `labels` is the
  // catalog's current label of cf_tier, absent when the catalog lacks the field.
  function catalogHarness(labels: { label?: string; de?: string }) {
    const seen: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        seen.push(url);
        const body = url.includes("/custom-fields")
          ? {
              data:
                labels.label === undefined
                  ? []
                  : [
                      {
                        column_name: "cf_tier",
                        label: labels.label,
                        label_translations: labels.de ? { de: labels.de } : {},
                        object: "contact",
                        type: "picklist",
                      },
                    ],
              page: { has_more: false },
            }
          : {
              resource: url.includes("resource=company")
                ? "company"
                : "contact",
              fields: [...VOCAB_BODY.fields, TIER],
            };
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return { seen, client };
  }

  function wrapperFor(client: QueryClient, locale: "en" | "de" = "en") {
    return ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial={locale}>{children}</LocaleProvider>
      </QueryClientProvider>
    );
  }

  const t = (key: MessageKey) => en[key];
  const tierLabel = (fields: readonly VocabularyField[] | undefined) => {
    const tier = fields?.find((field) => field.name === "cf_tier");
    return tier && fieldLabel(tier, t);
  };

  it("calls a custom field by the label its admin gave it", async () => {
    const { client } = catalogHarness({ label: "Loyalty Tier" });
    const { result } = renderHook(() => useFilterVocabulary("contact"), {
      wrapper: wrapperFor(client),
    });

    await waitFor(() =>
      expect(tierLabel(result.current.data?.fields)).toBe("Loyalty Tier"),
    );
  });

  it("follows a rename once the catalog is read again", async () => {
    const labels = { label: "Loyalty Tier" };
    const { client } = catalogHarness(labels);
    const { result } = renderHook(() => useFilterVocabulary("contact"), {
      wrapper: wrapperFor(client),
    });
    await waitFor(() =>
      expect(tierLabel(result.current.data?.fields)).toBe("Loyalty Tier"),
    );

    labels.label = "Customer Tier";
    await act(() =>
      client.invalidateQueries({ queryKey: ["custom-fields", "contact"] }),
    );

    await waitFor(() =>
      expect(tierLabel(result.current.data?.fields)).toBe("Customer Tier"),
    );
  });

  it("reads the label in the reader's language where the admin gave one", async () => {
    const { client } = catalogHarness({
      label: "Loyalty Tier",
      de: "Treuestufe",
    });
    const { result } = renderHook(() => useFilterVocabulary("contact"), {
      wrapper: wrapperFor(client, "de"),
    });

    await waitFor(() =>
      expect(tierLabel(result.current.data?.fields)).toBe("Treuestufe"),
    );
  });

  it("keeps the column's words for a field the catalog does not name", async () => {
    const { client } = catalogHarness({});
    const { result } = renderHook(() => useFilterVocabulary("contact"), {
      wrapper: wrapperFor(client),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
      expect(client.getQueryState(["custom-fields", "contact"])?.status).toBe(
        "success",
      );
    });
    expect(tierLabel(result.current.data?.fields)).toBe("tier");
  });

  it("asks no catalog for a kind an extension declared", async () => {
    const { seen, client } = catalogHarness({ label: "Loyalty Tier" });
    const { result } = renderHook(
      () => useFilterVocabulary("ext:fleet:vehicle"),
      { wrapper: wrapperFor(client) },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(seen.filter((url) => url.includes("/custom-fields"))).toEqual([]);
  });
});

describe("the preview read", () => {
  it("does not ask about an empty group", async () => {
    const { seen, wrapper } = harness();
    renderHook(() => useFilterPreview("contact", newGroup("and")), { wrapper });

    // An empty group is refused as filter_shape_invalid, so asking spends a
    // request to be told what isComplete already knows.
    await waitFor(() => expect(seen).toHaveLength(0));
  });

  it("does not ask about a clause whose value is still empty", async () => {
    const { seen, wrapper } = harness();
    renderHook(
      () =>
        useFilterPreview(
          "contact",
          newGroup("and", [newLeaf("full_name", "contains", "")]),
        ),
      { wrapper },
    );

    // The 422 this would earn names the field, which is correct of the server and
    // useless to somebody who has not finished typing.
    await waitFor(() => expect(seen).toHaveLength(0));
  });

  it("asks once a clause is complete", async () => {
    const { seen, wrapper } = harness();
    const { result } = renderHook(
      () =>
        useFilterPreview(
          "contact",
          newGroup("and", [newLeaf("full_name", "contains", "ann")]),
        ),
      { wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.match_count).toBe(7);
    expect(seen.filter((url) => url.includes("/filters/preview"))).toHaveLength(
      1,
    );
  });

  it("spends one request on two trees that encode the same filter", async () => {
    const { seen, wrapper } = harness();
    // Structurally identical, different node ids — which is exactly what an edit
    // that removes a clause and retypes it produces. Keying on the encoded tree
    // rather than on identity is what stops the second one costing a request to
    // learn the count did not move.
    const first = newGroup("and", [newLeaf("full_name", "contains", "ann")]);
    const second = newGroup("and", [newLeaf("full_name", "contains", "ann")]);
    expect(first.id).not.toBe(second.id);

    const { result, rerender } = renderHook(
      ({ tree }: { tree: ReturnType<typeof newGroup> }) =>
        useFilterPreview("contact", tree),
      { wrapper, initialProps: { tree: first } },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    rerender({ tree: second });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(seen.filter((url) => url.includes("/filters/preview"))).toHaveLength(
      1,
    );
  });
});

describe("paging the preview", () => {
  it("asks for page two with the first page's cursor and the same page size", async () => {
    const bodies: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : null;
        const body: unknown = request ? await request.json() : undefined;
        bodies.push(body);
        const second = bodies.length > 1;
        return new Response(
          JSON.stringify({
            resource: "contact",
            match_count: 3,
            columns: ["id"],
            rows: second ? [{ id: "c" }] : [{ id: "a" }, { id: "b" }],
            truncated: !second,
            next_cursor: second ? null : "b",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["me"], meFixture({ rowScope: "all" }));
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(
      () =>
        useFilterPreview(
          "contact",
          newGroup("and", [newLeaf("full_name", "contains", "ann")]),
        ),
      { wrapper },
    );

    await waitFor(() => expect(result.current.hasNextPage).toBe(true));
    await result.current.fetchNextPage();
    await waitFor(() => expect(result.current.data?.rows).toHaveLength(3));

    expect(bodies[0]).not.toHaveProperty("cursor");
    expect(bodies[1]).toMatchObject({ cursor: "b", limit: PREVIEW_PAGE });
    expect(result.current.data?.match_count).toBe(3);
    expect(result.current.hasNextPage).toBe(false);
  });
});

describe("presenting a vocabulary", () => {
  const t = (key: MessageKey) => en[key];
  const FIELDS: VocabularyField[] = [
    { name: "owner_id", type: "id", operators: ["eq"], custom: false },
    { name: "cf_tier", type: "text", operators: ["eq"], custom: true },
    { name: "full_name", type: "text", operators: ["eq"], custom: false },
  ];

  it("keeps core fields ahead of a workspace's own", () => {
    const { core, custom } = groupFields(FIELDS);
    expect(core.map((f) => f.name)).toEqual(["owner_id", "full_name"]);
    expect(custom.map((f) => f.name)).toEqual(["cf_tier"]);
  });

  it("reads a custom column as the admin's words, not the column's", () => {
    // cf_ is a Go-side convention a human never typed, and an underscore is a
    // column separator rather than something anybody wrote in a label.
    expect(
      fieldLabel(
        {
          name: "cf_loyalty_tier",
          type: "text",
          operators: [],
          custom: true,
        },
        t,
      ),
    ).toBe("loyalty tier");
    // A core field keeps its own name: it has no prefix to strip, and stripping
    // one that only LOOKS like a prefix would rename the field.
    expect(
      fieldLabel(
        {
          name: "cf_score",
          type: "number",
          operators: [],
          custom: false,
        },
        t,
      ),
    ).toBe("cf score");
  });

  it("names a core field in the reader's words, not its wire name", () => {
    // The vocabulary sends `owner_id`; a picker printing "owner id" spoke the
    // column's language to somebody who never sees a column.
    expect(fieldLabel(FIELDS[0], t)).toBe("Owner");
    expect(
      fieldLabel(
        { name: "owner_team_id", type: "id", operators: [], custom: false },
        t,
      ),
    ).toBe("Owner team");
  });

  it("never lends a core word to a custom column of the same shape", () => {
    expect(
      fieldLabel(
        { name: "owner_id", type: "text", operators: [], custom: true },
        t,
      ),
    ).toBe("owner id");
  });
});
