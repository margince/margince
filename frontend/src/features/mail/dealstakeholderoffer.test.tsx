/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import type { Relationship } from "../../components/relationships";
import { Button } from "../../design-system/atoms";
import { LocaleProvider } from "../../i18n";
import {
  stakeholderCandidates,
  useDealStakeholderOffer,
} from "./dealstakeholderoffer";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailParty = components["schemas"]["EmailParty"];

const partner: EmailParty = {
  address: "pat@partner.example",
  display_name: "Pat Partner",
  contact_id: "c-pat",
};
const second: EmailParty = {
  address: "sam@partner.example",
  display_name: "Sam Second",
  contact_id: "c-sam",
};
const seated: EmailParty = {
  address: "alice@acme.example",
  display_name: "Alice Acme",
  contact_id: "c-alice",
};
const colleague: EmailParty = {
  address: "rep@myco.example",
  display_name: "Rep",
  contact_id: "c-rep",
  user_id: "u-rep",
};
const stranger: EmailParty = { address: "nobody@else.example" };

function presentation(parties: Partial<EmailPresentation>): EmailPresentation {
  return {
    from: [],
    to: [],
    cc: [],
    bcc: [],
    ...parties,
  } as EmailPresentation;
}

function seat(contactId: string): Relationship {
  return { contact_id: contactId } as Relationship;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("stakeholderCandidates", () => {
  it("names the mail's outside contacts who hold no seat on the deal, once each", () => {
    const mail = presentation({
      from: [colleague],
      to: [partner, seated],
      cc: [stranger, partner, second],
    });
    expect(stakeholderCandidates(mail, [seat("c-alice")])).toEqual([
      { contactId: "c-pat", name: "Pat Partner" },
      { contactId: "c-sam", name: "Sam Second" },
    ]);
  });

  it("names nobody when everyone on the mail is seated or a colleague", () => {
    const mail = presentation({ from: [colleague], to: [seated] });
    expect(stakeholderCandidates(mail, [seat("c-alice")])).toEqual([]);
  });
});

type Sent = Readonly<{ key: string; body: unknown }>;

function stubServer(mail: EmailPresentation, seats: Relationship[]) {
  const sent: Sent[] = [];
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
      const body = method === "GET" ? null : await request?.clone().json();
      sent.push({ key, body });
      const json = (value: unknown, status = 200) =>
        new Response(JSON.stringify(value), {
          status,
          headers: { "Content-Type": "application/json" },
        });
      switch (key) {
        case "GET /activities/m-1/email-presentation":
          return json(mail);
        case "GET /relationships": {
          // One seat per page, so a seat past the first page is read too.
          const at = Number(url.searchParams.get("cursor") ?? "0");
          const more = at + 1 < seats.length;
          return json({
            data: seats.slice(at, at + 1),
            page: { has_more: more, next_cursor: more ? String(at + 1) : null },
          });
        }
        case "POST /relationships":
          return json({ id: "r-new", ...(body as object) }, 201);
        default:
          throw new Error(`no stub for ${key}`);
      }
    }),
  );
  return sent;
}

function Harness({ done }: Readonly<{ done: () => void }>) {
  const stakeholders = useDealStakeholderOffer();
  return (
    <>
      <Button onClick={() => void stakeholders.offer("m-1", "d-1", done)}>
        linked
      </Button>
      {stakeholders.element}
    </>
  );
}

function renderHarness(done: () => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Harness done={done} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const added = (sent: readonly Sent[]) =>
  sent.filter((request) => request.key === "POST /relationships");

describe("useDealStakeholderOffer", () => {
  it("adds every ticked contact as a stakeholder on the deal", async () => {
    const user = userEvent.setup();
    const sent = stubServer(presentation({ to: [partner, second, seated] }), [
      seat("c-alice"),
    ]);
    const done = vi.fn();
    renderHarness(done);

    await user.click(screen.getByRole("button", { name: "linked" }));
    await user.click(
      await screen.findByRole("checkbox", { name: "Sam Second" }),
    );
    expect(screen.queryByRole("checkbox", { name: "Alice Acme" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Add to deal" }));

    await waitFor(() => expect(done).toHaveBeenCalledOnce());
    expect(added(sent).map((request) => request.body)).toEqual([
      {
        kind: "deal_stakeholder",
        source: "manual",
        deal_id: "d-1",
        contact_id: "c-pat",
      },
    ]);
  });

  it("adds nobody when the offer is declined", async () => {
    const user = userEvent.setup();
    const sent = stubServer(presentation({ to: [partner] }), []);
    const done = vi.fn();
    renderHarness(done);

    await user.click(screen.getByRole("button", { name: "linked" }));
    await user.click(await screen.findByRole("button", { name: "Not now" }));

    await waitFor(() => expect(done).toHaveBeenCalledOnce());
    expect(added(sent)).toEqual([]);
  });

  it("asks nothing when everyone on the mail is already on the deal", async () => {
    const user = userEvent.setup();
    const sent = stubServer(
      presentation({ from: [colleague], to: [seated, partner] }),
      [seat("c-alice"), seat("c-pat")],
    );
    const done = vi.fn();
    renderHarness(done);

    await user.click(screen.getByRole("button", { name: "linked" }));

    await waitFor(() => expect(done).toHaveBeenCalledOnce());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(added(sent)).toEqual([]);
  });
});
