/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ListAction, TagAction } from "./companyactions";

// How a typed tag name resolves to ONE tag. The interesting behaviour is the
// request sequence — which reads happen before the write, and what the action
// does with the collisions the server can answer with — so these drive the
// real component and assert on the calls it makes.

type Call = { method: string; path: string };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const page = { has_more: false };

/** stub records every call and answers from `reply`. */
function stub(reply: (call: Call, nth: number) => Response) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const call = {
        method: request.method,
        path: new URL(request.url).pathname,
      };
      calls.push(call);
      return reply(call, calls.length);
    }),
  );
  return calls;
}

function Wrapper({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

/** Fills one of these single-field actions in and submits it. */
async function submitName(verb: string, action: ReactNode, name: string) {
  const user = userEvent.setup();
  render(<Wrapper>{action}</Wrapper>);
  await user.click(screen.getByRole("button", { name: verb }));
  await user.type(screen.getByRole("textbox"), name);
  await user.click(screen.getByRole("button", { name: "Create" }));
}

async function addTagNamed(name: string) {
  await submitName("Add tag", <TagAction orgId="o-1" />, name);
}

async function addToListNamed(name: string) {
  await submitName("Add to list", <ListAction orgId="o-1" />, name);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Add tag resolves a typed name to one tag", () => {
  it("reads the catalog at submit, so a retry reuses what the last attempt made", async () => {
    // The tag exists because a previous attempt created it and then failed to
    // apply it. Nothing the component loaded on mount knows that, so a
    // resolution reading a snapshot would create a second "Landlord".
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: [{ id: "t-1", name: "Landlord" }], page });
      }
      return json({ id: "tg-1" }, 201);
    });

    await addTagNamed("landlord");

    // No POST /tags at all: the existing tag was matched, case-insensitively.
    expect(
      calls.some((c) => c.method === "POST" && c.path.endsWith("/tags")),
    ).toBe(false);
    expect(calls.some((c) => c.path.includes("/apply"))).toBe(true);
  });

  it("resolves a create-time collision by reading the winner back", async () => {
    // Somebody created the same name between this action's read and its write.
    // tag names are unique per workspace, so their row IS the asked-for tag.
    let seenTags = [{ id: "t-1", name: "Other" }];
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: seenTags, page });
      }
      if (call.path.endsWith("/tags") && call.method === "POST") {
        seenTags = [...seenTags, { id: "t-9", name: "Landlord" }];
        return json({ title: "conflict" }, 409);
      }
      return json({ id: "tg-1" }, 201);
    });

    await addTagNamed("Landlord");

    // Read, failed create, read again, then apply the winner.
    expect(
      calls.filter((c) => c.method === "GET" && c.path.endsWith("/tags")),
    ).toHaveLength(2);
    expect(calls.at(-1)?.path).toContain("/tags/t-9/apply");
  });

  // The overflow signal, in the two directions it has. `page.has_more` on a
  // bounded catalog says the read was CUT, so a name it does not carry cannot
  // be told apart from one past the cap — and creating on that guess is what
  // hands the rep a 409 about a tag the same cap hides from them.
  it("refuses a create the catalog could not have ruled out", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({
          data: [{ id: "t-1", name: "Other" }],
          page: { has_more: true },
        });
      }
      return json({ id: "tg-1" }, 201);
    });

    await addTagNamed("Landlord");

    expect(
      await screen.findByText(/more tags than this list can show/i),
    ).toBeTruthy();
    expect(
      calls.some((c) => c.method === "POST" && c.path.endsWith("/tags")),
    ).toBe(false);
  });

  it("creates the new tag while the catalog fits inside its cap", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: [{ id: "t-1", name: "Other" }], page });
      }
      if (call.path.endsWith("/tags") && call.method === "POST") {
        return json({ id: "t-9", name: "Landlord" }, 201);
      }
      return json({ id: "tg-1" }, 201);
    });

    await addTagNamed("Landlord");

    expect(
      calls.some((c) => c.method === "POST" && c.path.endsWith("/tags")),
    ).toBe(true);
    expect(calls.at(-1)?.path).toContain("/tags/t-9/apply");
  });

  it("treats an already-applied tag as the state the rep asked for", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: [{ id: "t-1", name: "Landlord" }], page });
      }
      return json({ title: "conflict" }, 409);
    });

    await addTagNamed("Landlord");

    // The apply collided and the action did NOT surface it: the tag is on the
    // company, which is what was asked for.
    expect(calls.some((c) => c.path.includes("/apply"))).toBe(true);
    expect(screen.queryByText(/conflict/i)).toBeNull();
  });
});

// The same shape one catalog over: `/lists` is bounded and cursorless too, and
// `list` carries no uniqueness on its name — so an over-cap create there gets
// no 409 at all, just a second list nobody asked for under a name the page
// could not show.
describe("Add to list reads the same overflow signal", () => {
  it("refuses a create the catalog could not have ruled out", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/lists") && call.method === "GET") {
        return json({
          data: [{ id: "l-1", name: "Other", list_type: "static" }],
          page: { has_more: true },
        });
      }
      return json({ id: "l-9" }, 201);
    });

    await addToListNamed("Key accounts");

    expect(
      await screen.findByText(/more lists than can be shown/i),
    ).toBeTruthy();
    expect(
      calls.some((c) => c.method === "POST" && c.path.endsWith("/lists")),
    ).toBe(false);
  });

  it("creates the new list while the catalog fits inside its cap", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/lists") && call.method === "GET") {
        return json({
          data: [{ id: "l-1", name: "Other", list_type: "static" }],
          page,
        });
      }
      if (call.path.endsWith("/lists") && call.method === "POST") {
        return json({ id: "l-9" }, 201);
      }
      return json({ id: "m-1" }, 201);
    });

    await addToListNamed("Key accounts");

    expect(
      calls.some((c) => c.method === "POST" && c.path.endsWith("/lists")),
    ).toBe(true);
    expect(calls.at(-1)?.path).toContain("/lists/l-9/members");
  });
});
