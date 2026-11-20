// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { categoryCuts } from "./attachmenttile";

type Attachment = components["schemas"]["Attachment"];

function file(id: string, category: Attachment["category"]): Attachment {
  return {
    id,
    entity_type: "company",
    entity_id: "o-1",
    filename: `${id}.pdf`,
    category,
    source: "upload",
    captured_by: "human:u-1",
    created_at: "2026-08-01T09:00:00Z",
  };
}

describe("the category chips over a list of files", () => {
  it("adds a chip for each other category a file holds", () => {
    expect(
      categoryCuts([file("a", "contract"), file("b", "offer")], ["offer"], ""),
    ).toEqual(["offer", "contract"]);
  });

  // A chip that vanished while pressed would leave its filter on with no
  // control to take it off.
  it("keeps the pressed chip once no file holds its category", () => {
    expect(categoryCuts([file("b", "offer")], ["offer"], "contract")).toEqual([
      "offer",
      "contract",
    ]);
  });
});
