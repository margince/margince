/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { activities, draw, SPOKE, view } from "./spine.testkit";

afterEach(cleanup);

// A `nameOf` over a fixed table, keyed "<entity type>:<id>". That is the
// spelling the thread's own resolver uses, so a test cannot prove a lookup the
// record page could not perform.
function resolver(names: Readonly<Record<string, string>>) {
  return (entityType: string, entityId: string) =>
    names[`${entityType}:${entityId}`];
}

// A subject line says what a conversation was about, not who it was with.
// That half of "what happened" only a name resolves.
describe("who a conversation was with", () => {
  it("names the contact a conversation's links resolve to", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-1",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            links: [{ entity_type: "contact", entity_id: "p-dana" }],
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "contact" && entityId === "p-dana"
          ? "Dana Otieno"
          : undefined,
    );

    expect(screen.getByText(/Dana Otieno/)).toBeTruthy();
  });

  it("drops a contact link the resolver cannot name, rather than printing its id", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-2",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            links: [{ entity_type: "contact", entity_id: "p-unknown" }],
          },
        ]),
      }),
      () => undefined,
    );

    // No name resolved, so the detail line falls back to the kind alone,
    // never the raw id, which a reader cannot recognise.
    expect(screen.queryByText(/p-unknown/)).toBeNull();
    expect(screen.getByText("Renewal terms")).toBeTruthy();
  });

  it("names the first two contacts on a conversation and counts the rest", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "n-3",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            links: [
              { entity_type: "contact", entity_id: "p-1" },
              { entity_type: "contact", entity_id: "p-2" },
              { entity_type: "contact", entity_id: "p-3" },
              { entity_type: "contact", entity_id: "p-4" },
            ],
          },
        ]),
      }),
      (_entityType, entityId) =>
        ({
          "p-1": "Ada Lin",
          "p-2": "Bo Chen",
          "p-3": "Cy Reyes",
          "p-4": "Di Fisher",
        })[entityId],
    );

    // Copy taken from co.spine.andOthers: "{names} and {count} others".
    expect(screen.getByText(/Ada Lin, Bo Chen and 2 others/)).toBeTruthy();
    expect(screen.queryByText(/Cy Reyes/)).toBeNull();
  });

  it("names who the folded-away earlier conversations were with", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          { id: "e-5", kind: "email", subject: "Fifth", at: SPOKE },
          {
            id: "e-4",
            kind: "email",
            subject: "Fourth",
            at: "2026-08-10T09:00:00Z",
          },
          {
            id: "e-3",
            kind: "email",
            subject: "Third",
            at: "2026-08-05T09:00:00Z",
          },
          {
            id: "e-2",
            kind: "email",
            subject: "Second",
            at: "2026-07-20T09:00:00Z",
            links: [{ entity_type: "contact", entity_id: "p-early" }],
          },
          {
            id: "e-1",
            kind: "email",
            subject: "First",
            at: "2026-07-01T09:00:00Z",
            links: [{ entity_type: "contact", entity_id: "p-early" }],
          },
        ]),
      }),
      (_entityType, entityId) =>
        entityId === "p-early" ? "Early Adopter" : undefined,
    );

    expect(screen.getByText("2 earlier threads")).toBeTruthy();
    expect(screen.getByText("Early Adopter")).toBeTruthy();
  });
});

// A meeting is the one exchange where the record can name both sides: the
// colleague who held it and who they held it with. Everything else on the
// thread names only the other party, because a mail carries no field for
// which mailbox sent it.
describe("who held a meeting", () => {
  it("names both sides when the host and the contact both resolve", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-1",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-lena",
            links: [{ entity_type: "contact", entity_id: "p-dana" }],
          },
        ]),
      }),
      resolver({
        "user:u-lena": "Lena Fischer",
        "contact:p-dana": "Dana Otieno",
      }),
    );

    expect(screen.getByText("Lena Fischer met Dana Otieno")).toBeTruthy();
  });

  it("says who held it when nobody on the other side resolves to a name", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-2",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-lena",
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "user" && entityId === "u-lena"
          ? "Lena Fischer"
          : undefined,
    );

    expect(screen.getByText("Meeting held by Lena Fischer")).toBeTruthy();
  });

  it("falls back to the plain meeting line when the host id resolves to nothing", () => {
    // A host id nobody can put a name to is dropped rather than printed. The
    // contact side keeps the same rule: a reader cannot recognise a uuid.
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "h-3",
            kind: "meeting",
            subject: "Quarterly review",
            at: SPOKE,
            host_user_id: "u-unknown",
            links: [{ entity_type: "contact", entity_id: "p-dana" }],
          },
        ]),
      }),
      (entityType, entityId) =>
        entityType === "contact" && entityId === "p-dana"
          ? "Dana Otieno"
          : undefined,
    );

    expect(screen.getByText("Meeting with Dana Otieno")).toBeTruthy();
    expect(screen.queryByText(/u-unknown/)).toBeNull();
    expect(screen.queryByText(/met Dana Otieno/)).toBeNull();
  });

  // `host_user_id` is a column on every activity row, not only a meeting's.
  // An email that carries one was still sent rather than held, so it reads by
  // direction like every other mail, never as something somebody "met".
  it("reads an email by direction even when it carries a host", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "e-host",
            kind: "email",
            subject: "Renewal terms",
            at: SPOKE,
            host_user_id: "u-lena",
            direction: "outbound",
            links: [{ entity_type: "contact", entity_id: "p-dana" }],
          },
        ]),
      }),
      resolver({
        "user:u-lena": "Lena Fischer",
        "contact:p-dana": "Dana Otieno",
      }),
    );

    expect(screen.getByText("Email to Dana Otieno")).toBeTruthy();
    expect(screen.queryByText(/met Dana Otieno/)).toBeNull();
    expect(screen.queryByText(/Lena Fischer/)).toBeNull();
  });
});

// A stop names a conversation. When that conversation is mail the reader may
// read, naming it and opening it are the same act. The thread is where a
// reader asks "what was that about", and the answer is the message.
describe("a conversation the reader can open", () => {
  // Two conversations, so the older one is a stop that leads with its subject
  // rather than with "Last contact".
  const twoMails = (over: Record<string, unknown> = {}) =>
    view({
      last_outbound_at: SPOKE,
      activities: activities([
        {
          id: "a-new",
          kind: "email",
          subject: "About capacity",
          at: SPOKE,
          emailStatus: "team",
        },
        {
          id: "a-old",
          kind: "email",
          subject: "About scope",
          at: "2026-08-10T09:00:00Z",
          emailStatus: "team",
        },
      ]),
      ...over,
    });

  it("opens the newest message of the conversation it names", async () => {
    const opened: string[] = [];
    draw(twoMails(), undefined, (activityId) => opened.push(activityId));

    await userEvent.click(screen.getByRole("button", { name: /About scope/ }));

    // The conversation's newest message, which is the one it is dated and
    // titled by. A stop that opened some other message of the thread would
    // show the reader something other than what they pressed.
    expect(opened).toEqual(["a-old"]);
  });

  it("names a call without offering to open it", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "a-mail",
            kind: "email",
            subject: "About capacity",
            at: SPOKE,
            emailStatus: "team",
          },
          {
            id: "a-call",
            kind: "call",
            subject: "Quick check-in",
            at: "2026-08-10T09:00:00Z",
          },
        ]),
      }),
      undefined,
      () => {},
    );

    // There is no message behind a call to open, and drawing an envelope
    // beside one would claim a transport nobody checked.
    expect(screen.getByText("Quick check-in")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Quick check-in/ })).toBeNull();
  });

  it("names a withheld conversation without offering to open it", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        activities: activities([
          {
            id: "a-mail",
            kind: "email",
            subject: "About capacity",
            at: SPOKE,
            emailStatus: "team",
          },
          {
            id: "a-private",
            kind: "email",
            subject: "About the offer",
            at: "2026-08-10T09:00:00Z",
            emailStatus: "withheld",
          },
        ]),
      }),
      undefined,
      () => {},
    );

    // A control that opened a placeholder teaches a reader that citations do
    // not work, which costs more than the press it saves.
    expect(
      screen.queryByRole("button", { name: /About the offer/ }),
    ).toBeNull();
  });

  it("draws plain subjects when the host offers no drawer", () => {
    draw(twoMails());

    // The other direction: a host with nowhere to send a reader gets text, not
    // a control that leads nowhere.
    expect(screen.getByText("About scope")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /About scope/ })).toBeNull();
  });
});
