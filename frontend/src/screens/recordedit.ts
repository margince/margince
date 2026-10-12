import { api } from "../api/client";
import { ifMatch } from "../api/version";
import { unwrap } from "./common";
import { companyEditComparison } from "./companyform";
import { contactEditComparison } from "./contactformfields";
import { saveIndependentEdit } from "./independentedit";
import type { FieldRecord } from "./recordfields";

// Core and custom fields use the same conditional write and recovery policy.
export function saveRecordEdit(
  kind: "company" | "contact" | "deal" | "lead",
  original: FieldRecord,
  patch: Record<string, unknown>,
) {
  const opened = { id: original.id, original };
  switch (kind) {
    case "company":
      return saveIndependentEdit({
        opened,
        patch,
        project: companyEditComparison,
        read: async () => {
          return unwrap(
            await api.GET("/companies/{id}", {
              params: { path: { id: original.id } },
            }),
          );
        },
        write: async (body, version) => {
          return unwrap(
            await api.PATCH("/companies/{id}", {
              params: { path: { id: original.id }, ...ifMatch(version) },
              body,
            }),
          );
        },
      });
    case "contact":
      return saveIndependentEdit({
        opened,
        patch,
        project: contactEditComparison,
        groups: [["full_name", "first_name", "last_name"]],
        read: async () => {
          return unwrap(
            await api.GET("/contacts/{id}", {
              params: { path: { id: original.id } },
            }),
          );
        },
        write: async (body, version) => {
          return unwrap(
            await api.PATCH("/contacts/{id}", {
              params: { path: { id: original.id }, ...ifMatch(version) },
              body,
            }),
          );
        },
      });
    case "deal":
      return saveIndependentEdit({
        opened,
        patch,
        groups: [
          ["currency", "amount_minor", "expected_arr_minor"],
          ["partner_company_id", "partner_attribution"],
          ["company_id", "project_id"],
        ],
        read: async () => {
          return unwrap(
            await api.GET("/deals/{id}", {
              params: { path: { id: original.id } },
            }),
          );
        },
        write: async (body, version) => {
          return unwrap(
            await api.PATCH("/deals/{id}", {
              params: { path: { id: original.id }, ...ifMatch(version) },
              body,
            }),
          );
        },
      });
    case "lead":
      return saveIndependentEdit({
        opened,
        patch,
        read: async () => {
          return unwrap(
            await api.GET("/leads/{id}", {
              params: { path: { id: original.id } },
            }),
          );
        },
        write: async (body, version) => {
          return unwrap(
            await api.PATCH("/leads/{id}", {
              params: { path: { id: original.id }, ...ifMatch(version) },
              body,
            }),
          );
        },
      });
  }
}
