// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import {
  archivedContactRow,
  contactPageState,
  relatedContactsQuery,
} from "./contact.queries";

describe("the contacts around a contact", () => {
  it("asks nothing until the contact is known", () => {
    expect(relatedContactsQuery(undefined).queryFn).toBe(skipToken);
  });

  it("keeps each contact's circle under its own key", () => {
    expect(relatedContactsQuery("p-1").queryKey).toEqual([
      "contactsRelatedTo",
      "p-1",
    ]);
    expect(relatedContactsQuery("p-1").queryFn).not.toBe(skipToken);
  });
});

describe("the contact page's read state", () => {
  const row = { isPending: false, isError: false, isFetching: false };
  const view = { isLoading: false, isError: false };

  it("calls a failed row unopened even while the 360 still loads", () => {
    expect(
      contactPageState({ ...row, isError: true }, { ...view, isLoading: true }),
    ).toBe("notOpened");
  });

  it("keeps opening while either read is pending", () => {
    expect(contactPageState({ ...row, isPending: true }, view)).toBe("opening");
    expect(contactPageState(row, { ...view, isLoading: true })).toBe("opening");
  });

  it("waits for a row re-read before a failed 360 counts", () => {
    expect(
      contactPageState(
        { ...row, isFetching: true },
        { ...view, isError: true },
      ),
    ).toBe("ready");
    expect(contactPageState(row, { ...view, isError: true })).toBe("notOpened");
  });

  it("opens the archived page only from a row that did not fail", () => {
    const archived = { archived_at: "2026-06-01T08:00:00Z" };
    expect(archivedContactRow({ isError: false, data: archived })).toBe(
      archived,
    );
    expect(
      archivedContactRow({ isError: true, data: archived }),
    ).toBeUndefined();
    expect(
      archivedContactRow({ isError: false, data: { archived_at: null } }),
    ).toBeUndefined();
  });
});
