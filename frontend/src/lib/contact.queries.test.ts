// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import {
  archivedContactRow,
  confirmedContactRow,
  contactPageState,
  openingIdentity,
  relatedContactsQuery,
} from "./contact.queries";

describe("the contacts around a contact", () => {
  it("asks nothing until the contact is known", () => {
    expect(relatedContactsQuery(undefined, "reader").queryFn).toBe(skipToken);
  });

  it("keeps each contact's circle under its own key", () => {
    expect(relatedContactsQuery("p-1", "reader").queryKey).toEqual([
      "contactsRelatedTo",
      "p-1",
      "reader",
    ]);
    expect(relatedContactsQuery("p-1", "reader").queryFn).not.toBe(skipToken);
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

  it("shows no record while a failed 360 waits on a row re-read", () => {
    expect(
      contactPageState(
        { ...row, isFetching: true },
        { ...view, isError: true },
      ),
    ).toBe("opening");
    expect(contactPageState(row, { ...view, isError: true })).toBe("notOpened");
  });

  const archived = { archived_at: "2026-06-01T08:00:00Z" };
  const read = {
    isError: false,
    isFetching: false,
    isFetchedAfterMount: false,
  };

  it("opens the archived page only from a row that did not fail", () => {
    expect(archivedContactRow({ ...read, data: archived })).toBe(archived);
    expect(
      archivedContactRow({ ...read, isError: true, data: archived }),
    ).toBeUndefined();
    expect(
      archivedContactRow({ ...read, data: { archived_at: null } }),
    ).toBeUndefined();
  });

  it("does not trust a cached row while its first re-read is in flight", () => {
    const pending = { ...read, isFetching: true, data: archived };
    expect(confirmedContactRow(pending)).toBeUndefined();
    expect(archivedContactRow(pending)).toBeUndefined();
    expect(contactPageState({ ...row, ...pending }, view)).toBe("opening");
    const later = { ...pending, isFetchedAfterMount: true };
    expect(confirmedContactRow(later)).toBe(archived);
  });

  it("names nobody in the opening after a refused 360 or for an untrusted archive", () => {
    const live = { ...read, data: { archived_at: null } };
    expect(openingIdentity(live, { isError: true })).toEqual({
      hideIdentity: true,
    });
    expect(
      openingIdentity({ ...read, isFetching: true, data: archived }, view),
    ).toEqual({ hideIdentity: true });
    expect(openingIdentity(live, view).contact).toBe(live.data);
  });
});
