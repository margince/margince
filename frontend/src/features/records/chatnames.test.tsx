/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { entityNameObserverQuery } from "../../components/entityref.queries";
import type { TimelineEntry } from "../../design-system/timelineentry";
import { translate } from "../../i18n";
import { useChatNames } from "./chatnames";

type Activity = components["schemas"]["Activity"];

const t = ((
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values)) as never;

function activity(id: string, kind: string, contactId: string): Activity {
  return {
    id,
    kind,
    occurred_at: "2026-11-20T10:00:00Z",
    occurred_precision: "instant",
    links: [
      {
        id: `l-${id}`,
        activity_id: id,
        entity_type: "contact",
        entity_id: contactId,
      },
    ],
    source: "manual",
    is_done: false,
    offers_email: false,
    created_at: "2026-11-20T10:00:00Z",
    updated_at: "2026-11-20T10:00:00Z",
    captured_by: "human:u-1",
  } as Activity;
}

const entry = (id: string, extra: Partial<TimelineEntry> = {}) =>
  ({
    id,
    kind: "message",
    provenance: { kind: "unknown" },
    ...extra,
  }) as TimelineEntry;

function namer(activities: Activity[], known: Record<string, string>) {
  const client = new QueryClient();
  for (const [id, name] of Object.entries(known)) {
    const query = entityNameObserverQuery("contact", id);
    client.setQueryData(query.queryKey ?? [], name);
  }
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useChatNames(activities, t, "en"), { wrapper }).result
    .current;
}

// A chat is filed on the contact it was with, so the thread names that
// contact as the other side instead of "Them".
describe("the other side of a chat message", () => {
  it("is the contact the chat is filed on", () => {
    const chat = activity("m1", "message", "c-joshua");
    const named = namer([chat], { "c-joshua": "Joshua Schrijvers" })(
      entry("m1"),
      chat,
    );
    expect(named.counterparts).toBe("Joshua Schrijvers");
    expect(named.counterpartContacts).toEqual([
      { key: "c-joshua", name: "Joshua Schrijvers" },
    ]);
  });

  it("leaves a mail, a named row, a withheld row and an unnamed contact as they were", () => {
    const mail = activity("e1", "email", "c-joshua");
    const chat = activity("m1", "message", "c-joshua");
    const stranger = activity("m2", "message", "c-unknown");
    const name = namer([mail, chat, stranger], {
      "c-joshua": "Joshua Schrijvers",
    });
    expect(
      name(entry("e1", { kind: "email" }), mail).counterparts,
    ).toBeUndefined();
    expect(name(entry("m1", { counterparts: "Ana" }), chat).counterparts).toBe(
      "Ana",
    );
    expect(
      name(entry("m1", { withheld: true }), chat).counterparts,
    ).toBeUndefined();
    expect(name(entry("m2"), stranger).counterparts).toBeUndefined();
  });
});
