// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { WritableRecord } from "./capability";
import { contactQuery } from "./contact.queries";
import { leadKey } from "./leadkeys";
import { unwrap } from "./problem";

type ReadableRecordType = Extract<
  components["schemas"]["CreateRecordGrantRequest"]["record_type"],
  "contact" | "company" | "deal" | "lead" | "project"
>;

type RecordRead = Readonly<{
  queryKey: QueryKey;
  queryFn: () => Promise<WritableRecord>;
}>;

// Each record under the key its own page reads it by. Every write to it
// invalidates that key (lib/recordwritekeys.ts), so the screen never holds a
// `writable` the record has since lost.
export function recordRead(kind: ReadableRecordType, id: string): RecordRead {
  const params = { params: { path: { id } } };
  switch (kind) {
    case "contact":
      return {
        queryKey: contactQuery(id).queryKey,
        queryFn: async () => unwrap(await api.GET("/contacts/{id}", params)),
      };
    case "company":
      return {
        queryKey: ["company", id],
        queryFn: async () => unwrap(await api.GET("/companies/{id}", params)),
      };
    case "deal":
      return {
        queryKey: ["deal", id],
        queryFn: async () => unwrap(await api.GET("/deals/{id}", params)),
      };
    case "lead":
      return {
        queryKey: leadKey(id),
        queryFn: async () => unwrap(await api.GET("/leads/{id}", params)),
      };
    case "project":
      return {
        queryKey: ["project", id],
        queryFn: async () => unwrap(await api.GET("/projects/{id}", params)),
      };
    default:
      // The route admits only the kinds above (share.tsx), so this is a kind a
      // newer contract added and this build cannot read.
      throw new Error(`no record read for kind ${kind}`);
  }
}
