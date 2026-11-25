/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { ToastProvider } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { useViewOnlyAccess, ViewOnly } from "./viewonly";

type Contact = components["schemas"]["Contact"];

const colleagues: Contact = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 7,
  owner_id: "u-owner",
  writable: false,
};

const ASK = en["editAccess.ask"].replace("{owner}", "Mira Voss");

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// The viewer may change contacts in general, the owner's name, and no open
// request yet.
function serve(seat: "full" | "read" = "full") {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      if (key === "GET /me") {
        return json({
          user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
          authorization: {
            seat_type: seat,
            objects: { contact: { read: true, update: true } },
          },
        });
      }
      if (key === "GET /users/names") {
        return json({ data: [{ id: "u-owner", display_name: "Mira Voss" }] });
      }
      return json(null);
    }),
  );
}

function draw(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>{node}</ToastProvider>
    </QueryClientProvider>,
  );
}

function Ownership({ contact }: Readonly<{ contact: Contact }>) {
  const access = useViewOnlyAccess("contact", contact);
  return (
    <RecordDetails
      sections={[
        {
          key: "ownership",
          label: "Ownership",
          rows: [{ key: "owner", term: "Owner", value: "Mira Voss" }],
        },
      ]}
      access={access}
    />
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ViewOnly — the head's pill", () => {
  it("names the owner and carries the one filled ask, the handover as a link", async () => {
    const user = userEvent.setup();
    serve();
    draw(
      <ViewOnly
        kind="contact"
        record={colleagues}
        name="Dana Buyer"
        reasonId="why"
      />,
    );
    await user.click(
      await screen.findByRole("button", {
        name: new RegExp(en["viewOnly.label"]),
      }),
    );
    const panel = await screen.findByRole("dialog");
    expect(
      await within(panel).findByText("Mira Voss owns Dana Buyer"),
    ).toBeTruthy();
    expect(
      within(panel).getByText(en["contact.notYoursToChange"]),
    ).toBeTruthy();
    const ask = await within(panel).findByRole("button", { name: ASK });
    const handover = await within(panel).findByRole("button", {
      name: en["handover.ask"].replace("{owner}", "Mira Voss"),
    });
    expect(ask.className).toContain("btn-primary");
    expect(handover.className).toContain("btn-link");
    expect(panel.querySelectorAll(".btn-primary")).toHaveLength(1);
  });

  it("keeps the refusal under the id the locked controls point at", async () => {
    serve();
    draw(
      <ViewOnly
        kind="contact"
        record={colleagues}
        name="Dana Buyer"
        reasonId="why"
      />,
    );
    await screen.findByRole("button", {
      name: new RegExp(en["viewOnly.label"]),
    });
    expect(document.getElementById("why")?.textContent).toBe(
      en["contact.notYoursToChange"],
    );
  });

  it.each<[string, Partial<Contact>]>([
    ["a record the reader may change", { writable: true, owner_id: "u1" }],
    ["an archived record, whose band says why", { archived_at: "2026-08-02" }],
  ])("draws nothing for %s", async (_, row) => {
    serve();
    draw(
      <>
        <ViewOnly
          kind="contact"
          record={{ ...colleagues, ...row }}
          name="Dana Buyer"
          reasonId="why"
        />
        <Ownership contact={{ ...colleagues, ...row }} />
      </>,
    );
    await screen.findByText("Mira Voss");
    expect(
      screen.queryByRole("button", { name: new RegExp(en["viewOnly.label"]) }),
    ).toBeNull();
    expect(screen.queryByText(en["record.details.access"])).toBeNull();
  });
});

describe("ViewOnly — the Ownership row", () => {
  it("adds Access: View only to Ownership, with the ask as a ghost", async () => {
    serve();
    draw(<Ownership contact={colleagues} />);
    const group = screen.getByRole("region", { name: "Ownership" });
    expect(
      await within(group).findByText(en["record.details.access"]),
    ).toBeTruthy();
    expect(within(group).getByText(en["viewOnly.pill"])).toBeTruthy();
    const ask = await within(group).findByRole("button", { name: ASK });
    expect(ask.className).toContain("btn-ghost");
  });

  it("says View only to a read seat, with no owner to ask", async () => {
    serve("read");
    draw(<Ownership contact={{ ...colleagues, owner_id: "u1" }} />);
    const group = screen.getByRole("region", { name: "Ownership" });
    expect(await within(group).findByText(en["viewOnly.pill"])).toBeTruthy();
    expect(within(group).queryByRole("button", { name: ASK })).toBeNull();
  });
});
