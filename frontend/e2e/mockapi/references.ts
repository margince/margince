// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import { calledContact } from "./calls";
import { anna, brandt, busyStageDeals, deals, seededLead } from "./fixtures";
import {
  createdLead,
  importedCompany,
  importedContact,
  jonas,
  newCompany,
  newContact,
} from "./records";
import type { Handler } from "./server";

type Reference = components["schemas"]["RecordReference"];

function reference(
  row: { id: string; archived_at?: string | null; logo_url?: string | null },
  name: string | null,
  company = false,
): Reference {
  return {
    id: row.id,
    name,
    archived: Boolean(row.archived_at),
    ...(company ? { logo: row.logo_url ?? null } : {}),
  };
}

export const recordReferences: Handler = ({ url, json }, state) => {
  const kind = url.searchParams.get("kind");
  let rows: Reference[];
  switch (kind) {
    case "contact":
      rows = [anna, calledContact, jonas, newContact, importedContact].map(
        (row) => reference(row, row.full_name),
      );
      break;
    case "company":
      rows = [brandt, newCompany, importedCompany].map((row) =>
        reference(row, row.display_name, true),
      );
      break;
    case "lead":
      rows = [seededLead, createdLead].map((row) =>
        reference(row, row.full_name?.trim() || row.email?.trim() || null),
      );
      break;
    case "deal":
      rows = [...deals, ...(state.options.busyStage ? busyStageDeals : [])].map(
        (row) => {
          const current = { ...row, ...state.dealPatches[row.id] };
          return reference(current, current.name);
        },
      );
      break;
    case "project":
      rows = state.projectState.projects.map((row) => reference(row, row.name));
      break;
    default:
      return json({ title: "Invalid record kind", status: 422 }, 422);
  }
  const requested = url.searchParams.getAll("id");
  if (requested.length < 1 || requested.length > 100) {
    return json({ title: "Invalid record count", status: 422 }, 422);
  }
  const found = new Map(rows.map((row) => [row.id, row]));
  return json({
    data: [...new Set(requested)].flatMap((id) => {
      const row = state.recordReferences.get(`${kind}:${id}`) ?? found.get(id);
      if (!row) return [];
      const patch = kind === "deal" ? state.dealPatches[id] : undefined;
      return [
        {
          ...row,
          ...(patch?.name !== undefined ? { name: patch.name } : {}),
          ...(patch?.archived_at !== undefined
            ? { archived: Boolean(patch.archived_at) }
            : {}),
        },
      ];
    }),
  });
};
