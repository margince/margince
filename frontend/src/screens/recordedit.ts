import { api } from "../api/client";
import { ifMatch } from "../api/version";
import { throwProblem } from "./common";
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
          const { data, error } = await api.GET("/companies/{id}", {
            params: { path: { id: original.id } },
          });
          if (error) throwProblem(error);
          return data;
        },
        write: async (body, version) => {
          const { data, error } = await api.PATCH("/companies/{id}", {
            params: { path: { id: original.id }, ...ifMatch(version) },
            body,
          });
          if (error) throwProblem(error);
          return data;
        },
      });
    case "contact":
      return saveIndependentEdit({
        opened,
        patch,
        project: contactEditComparison,
        groups: [["full_name", "first_name", "last_name"]],
        read: async () => {
          const { data, error } = await api.GET("/contacts/{id}", {
            params: { path: { id: original.id } },
          });
          if (error) throwProblem(error);
          return data;
        },
        write: async (body, version) => {
          const { data, error } = await api.PATCH("/contacts/{id}", {
            params: { path: { id: original.id }, ...ifMatch(version) },
            body,
          });
          if (error) throwProblem(error);
          return data;
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
          const { data, error } = await api.GET("/deals/{id}", {
            params: { path: { id: original.id } },
          });
          if (error) throwProblem(error);
          return data;
        },
        write: async (body, version) => {
          const { data, error } = await api.PATCH("/deals/{id}", {
            params: { path: { id: original.id }, ...ifMatch(version) },
            body,
          });
          if (error) throwProblem(error);
          return data;
        },
      });
    case "lead":
      return saveIndependentEdit({
        opened,
        patch,
        read: async () => {
          const { data, error } = await api.GET("/leads/{id}", {
            params: { path: { id: original.id } },
          });
          if (error) throwProblem(error);
          return data;
        },
        write: async (body, version) => {
          const { data, error } = await api.PATCH("/leads/{id}", {
            params: { path: { id: original.id }, ...ifMatch(version) },
            body,
          });
          if (error) throwProblem(error);
          return data;
        },
      });
  }
}
