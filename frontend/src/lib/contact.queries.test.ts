// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { relatedContactsQuery } from "./contact.queries";

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
