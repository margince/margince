// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { AccessPreviewPanel } from "./users-access";

// What the User access preview says a role reads and changes, worded from the
// grants the server evaluated. Row scope says whose records a write reaches; it
// never says the role holds a write.

type Grant = Readonly<{
  read?: boolean;
  create?: boolean;
  update?: boolean;
  delete?: boolean;
}>;

const REP: Parameters<typeof AccessPreviewPanel>[0]["role"] = "rep";

function stubPreview(
  preview: Readonly<{ row_scope: string; objects?: Record<string, Grant> }>,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            role: "rep",
            objects: {},
            field_masks: [],
            teams: [],
            ...preview,
          }),
          { headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
}

function Providers({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AccessPreviewPanel for a role that changes nothing", () => {
  // The seeded Read-only role: row scope `all`, every record kind read, no
  // write. The scope says whose records a write would reach; with no write it
  // reaches none.
  it("says a read-only role changes no records, whatever its row scope", async () => {
    stubPreview({
      row_scope: "all",
      objects: {
        contact: { read: true },
        company: { read: true },
        lead: { read: true },
        deal: { read: true },
        project: { read: true },
      },
    });
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={[]} />
      </Providers>,
    );

    expect(await screen.findByText(en["users.access.writesNone"])).toBeTruthy();
    expect(screen.queryByText(en["users.access.writesAll"])).toBeNull();
  });

  // Creating a record changes none that exists, so a create-only role with
  // scope `all` is not said to edit every record.
  it("says a role that may only create changes no records", async () => {
    stubPreview({
      row_scope: "all",
      objects: { lead: { read: true, create: true } },
    });
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={[]} />
      </Providers>,
    );

    expect(await screen.findByText(en["users.access.writesNone"])).toBeTruthy();
    expect(screen.queryByText(en["users.access.writesAll"])).toBeNull();
  });

  it("leaves deals out of what a role reads when its deal read is off", async () => {
    stubPreview({
      row_scope: "own",
      objects: {
        contact: { read: true, update: true },
        company: { read: true },
      },
    });
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={[]} />
      </Providers>,
    );

    expect(
      await screen.findByText(
        en["users.access.reads"].replace("{kinds}", "contacts and companies"),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        `${en["users.access.object.deal"]}: ${en["users.access.none"]}`,
      ),
    ).toBeTruthy();
  });

  it("says a role that reads no record kind reads nothing", async () => {
    stubPreview({ row_scope: "all" });
    render(
      <Providers>
        <AccessPreviewPanel role={REP} teamIds={[]} />
      </Providers>,
    );

    expect(await screen.findByText(en["users.access.readsNone"])).toBeTruthy();
    expect(screen.getByText(en["users.access.writesNone"])).toBeTruthy();
  });
});
