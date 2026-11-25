/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { useMe } from "./session";
import {
  useActionRefusal,
  useArchiveRefusal,
  useKindWriteRefusal,
} from "./writerefusal";

function stubMe(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

async function settled<T>(read: () => T): Promise<T> {
  const { result } = renderHook(() => ({ me: useMe(), value: read() }), {
    wrapper,
  });
  await waitFor(() => {
    expect(result.current.me.isPending).toBe(false);
  });
  return result.current.value;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useKindWriteRefusal", () => {
  it("tells a role that only reads to ask for another role, on every kind", async () => {
    stubMe(meFixture({ allow: { company: ["read"], lead: ["read"] } }));

    for (const kind of ["company", "lead"] as const) {
      expect(
        await settled(() => useKindWriteRefusal(kind, { writable: false })),
      ).toBe(en["record.roleReadsOnly"]);
    }
  });

  it("tells a read seat to ask for a full seat, whatever the role holds", async () => {
    stubMe(meFixture({ allow: { company: ["read", "update"] }, seat: "read" }));

    expect(
      await settled(() => useKindWriteRefusal("company", { writable: false })),
    ).toBe(en["settings.seatRefused"]);
  });

  it("keeps the owner sentence for a writing role on somebody else's row", async () => {
    stubMe(meFixture({ allow: { company: ["read", "update"] } }));

    expect(
      await settled(() => useKindWriteRefusal("company", { writable: false })),
    ).toBe(en["record.notYoursToChange"]);
  });
});

describe("useActionRefusal", () => {
  it("tells a role that only reads to ask for another role", async () => {
    stubMe(meFixture({ allow: { contact: ["read"] } }));

    expect(
      await settled(() =>
        useActionRefusal({ object: "contact", action: "create" }),
      ),
    ).toBe(en["record.roleReadsOnly"]);
  });

  it("names the seat on a read seat, whatever the role holds", async () => {
    stubMe(
      meFixture({
        allow: { contact: ["read", "create", "update"] },
        seat: "read",
      }),
    );

    expect(
      await settled(() =>
        useActionRefusal({ object: "contact", action: "create" }),
      ),
    ).toBe(en["settings.seatRefused"]);
  });

  it("names the missing right on a role that writes but lacks this grant", async () => {
    stubMe(meFixture({ allow: { contact: ["read", "update"] } }));

    expect(
      await settled(() =>
        useActionRefusal({ object: "contact", action: "create" }),
      ),
    ).toBe(en["record.actionRefused"]);
  });

  it("refuses nothing a role holds", async () => {
    stubMe(meFixture({ allow: { contact: ["read", "create"] } }));

    expect(
      await settled(() =>
        useActionRefusal({ object: "contact", action: "create" }),
      ),
    ).toBeUndefined();
  });
});

describe("useArchiveRefusal", () => {
  it("tells a role that only reads to ask for another role", async () => {
    stubMe(meFixture({ allow: { deal: ["read"], project: ["read"] } }));

    for (const kind of ["deal", "project"] as const) {
      expect(await settled(() => useArchiveRefusal(kind))).toBe(
        en["record.roleReadsOnly"],
      );
    }
  });

  it("names the seat on a read seat", async () => {
    stubMe(meFixture({ allow: { contact: ["read", "update"] }, seat: "read" }));

    expect(await settled(() => useArchiveRefusal("contact"))).toBe(
      en["settings.seatRefused"],
    );
  });

  it("sends a writing role on a contact, company or deal to its owner or a manager", async () => {
    stubMe(
      meFixture({
        allow: {
          contact: ["read", "update"],
          company: ["read", "update"],
          deal: ["read", "update"],
        },
      }),
    );

    for (const kind of ["contact", "company", "deal"] as const) {
      expect(await settled(() => useArchiveRefusal(kind))).toBe(
        en["record.archiveRefused"],
      );
    }
  });

  it("names the missing right on a project, which its owner cannot archive either", async () => {
    stubMe(meFixture({ allow: { project: ["read", "update"] } }));

    expect(await settled(() => useArchiveRefusal("project"))).toBe(
      en["record.actionRefused"],
    );
  });
});
