/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { TagAction } from "./companyactions";

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
      <LocaleProvider>
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject includes what an apply SAYS mounts it the same
            way — the Undo this action offers lives inside it. */}
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
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
  return user;
}

async function addTagNamed(name: string) {
  return submitName("Add tag", <TagAction orgId="o-1" />, name);
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

  // The way back, which is what makes this the one destructive-adjacent verb on
  // the company page allowed to say Undo: `DELETE /tags/{id}/apply` is the
  // contract's own inverse of the apply, idempotent and complete.
  it("takes the tag back off through the Undo the confirmation carries", async () => {
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: [{ id: "t-1", name: "Landlord" }], page });
      }
      return json({ id: "tg-1" }, 201);
    });

    const user = await addTagNamed("Landlord");

    const said = await screen.findByRole("status");
    expect(said).toHaveTextContent(
      en["co.tags.applied"].replace("{name}", "Landlord"),
    );
    await user.click(
      within(said).getByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "DELETE")).toBe(true),
    );
    expect(calls.at(-1)?.path).toContain("/tags/t-1/apply");
    expect(await screen.findByRole("status")).toHaveTextContent(
      en["co.tags.removed"].replace("{name}", "Landlord"),
    );
  });

  it("says so when taking the tag back off is refused", async () => {
    // The message the Undo was offered from is consumed by the press, so a
    // silent refusal leaves the reader watching a confirmation disappear and
    // believing the tag came off.
    const calls = stub((call) => {
      if (call.path.endsWith("/tags") && call.method === "GET") {
        return json({ data: [{ id: "t-1", name: "Landlord" }], page });
      }
      if (call.method === "DELETE") {
        return json({ detail: "the tag is enforced by a rule" }, 409);
      }
      return json({ id: "tg-1" }, 201);
    });

    const user = await addTagNamed("Landlord");
    await user.click(
      within(await screen.findByRole("status")).getByRole("button", {
        name: en["common.undo"],
      }),
    );

    expect(
      await screen.findByText("the tag is enforced by a rule"),
    ).toBeTruthy();
    expect(calls.some((call) => call.method === "DELETE")).toBe(true);
  });
});
