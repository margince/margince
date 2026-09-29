// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { refreshAfterRoleEdit, roleLabel } from "./roles.queries";

const t = (key: Parameters<typeof translate>[1]) => translate("de", key);

describe("roleLabel", () => {
  const label = roleLabel(t);

  it("reads a seeded role under its seeded name in the reader's language", () => {
    expect(label("rep", "User")).toBe(translate("de", "role.rep"));
    expect(label("rep")).toBe(translate("de", "role.rep"));
  });

  it("reads a renamed seeded role under the name the operator gave it", () => {
    expect(label("rep", "Account executive")).toBe("Account executive");
  });

  it("reads a made role under its name, and an unnamed key as itself", () => {
    expect(label("custom_closers", "Closers")).toBe("Closers");
    expect(label("custom_closers")).toBe("custom_closers");
  });
});

describe("refreshAfterRoleEdit", () => {
  // Each member's allowed actions follow the roles they and the reader hold,
  // so both roster reads go stale with a role edit.
  it("marks the rosters, the directory and the reader's own grants stale", async () => {
    const client = new QueryClient();
    const keys = [
      ["users-admin"],
      ["users", "page"],
      ["roles", "live"],
      ["me"],
      ["access-preview", "rep", ""],
    ];
    for (const key of keys) {
      client.setQueryData(key, []);
    }
    client.setQueryData(["deals"], []);

    await refreshAfterRoleEdit(client);

    for (const key of keys) {
      expect(client.getQueryState(key)?.isInvalidated, key.join("/")).toBe(
        true,
      );
    }
    expect(client.getQueryState(["deals"])?.isInvalidated).toBe(false);
  });
});
