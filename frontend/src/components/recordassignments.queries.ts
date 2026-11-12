import {
  skipToken,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { throwProblem } from "../lib/problem";
import { shippedLabel, withShippedLabels } from "../lib/shippedlabels";

// Who is responsible for a record, and the administered roles they hold it
// under. The record panel, the settings card and the role picker all read the
// vocabulary through here, so they cannot disagree about what it holds or
// which roles are still choosable.
//
// An assignment grants nobody access. It records responsibility; visibility
// stays with ownership and record grants, and the panel says so in its own
// help text because the two are easy to confuse.

export type RecordRole = components["schemas"]["RecordRole"];
export type RecordAssignment = components["schemas"]["RecordAssignment"];
export type AssignmentRecordType =
  components["schemas"]["AssignmentRecordType"];
export type AssignmentSubjectKind =
  components["schemas"]["AssignmentSubjectKind"];

export const RECORD_ROLES_KEY = ["record-roles"] as const;

export function recordAssignmentsKey(
  recordType: AssignmentRecordType,
  recordId: string,
) {
  return ["record-assignments", recordType, recordId] as const;
}

export function useRecordRoles() {
  const t = useT();
  return useQuery({
    queryKey: RECORD_ROLES_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/record-roles");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (roles) => withShippedLabels(roles, t),
  });
}

/**
 * The roles a picker may offer for one record.
 *
 * Retired roles are dropped and inapplicable ones are dropped: a role the
 * server would refuse should not be offerable, or the first a user learns of
 * the rule is a failed save. Existing assignments keep rendering their role
 * label regardless, which is a different question answered on the row itself.
 */
export function assignableRoles(
  roles: RecordRole[] | undefined,
  recordType: AssignmentRecordType,
  subjectKind?: AssignmentSubjectKind,
): RecordRole[] {
  return (roles ?? []).filter(
    (role) =>
      role.active &&
      role.record_types.includes(recordType) &&
      (subjectKind === undefined || role.assignee_kinds.includes(subjectKind)),
  );
}

function recordAssignmentsQuery(
  recordType: AssignmentRecordType,
  recordId: string | undefined,
) {
  return {
    queryKey: recordAssignmentsKey(recordType, recordId ?? ""),
    queryFn: recordId
      ? async () => {
          const { data, error, response } = await api.GET(
            "/records/{record_type}/{record_id}/assignments",
            {
              params: {
                path: { record_type: recordType, record_id: recordId },
              },
            },
          );
          if (error || !response.ok) {
            throwProblem(error);
          }
          return data.data;
        }
      : skipToken,
  };
}

export function useRecordAssignments(
  recordType: AssignmentRecordType,
  recordId: string | undefined,
) {
  return useQuery(recordAssignmentsQuery(recordType, recordId));
}

/** A record a piece of work is about, as an assignment can name it. */
export type AssignedRecord = Readonly<{
  type: AssignmentRecordType;
  id: string;
}>;

const ASSIGNABLE_RECORD_TYPES: readonly AssignmentRecordType[] = [
  "company",
  "deal",
  "project",
];

/**
 * The record behind one entity reference, when it is a kind that names
 * colleagues with roles; nothing for a contact or a lead.
 */
export function assignedRecord(
  type: string | undefined,
  id: string | undefined,
): AssignedRecord[] {
  const kind = ASSIGNABLE_RECORD_TYPES.find((known) => known === type);
  return kind && id ? [{ type: kind, id }] : [];
}

/**
 * The colleagues named on any of these records, each with the roles they hold
 * there, in the reader's language. Teams and inactive members are left out:
 * only a colleague can be handed a task. A failed read names nobody, and the
 * picker keeps its usual order.
 */
export function useNamedColleagues(
  records: readonly AssignedRecord[],
): ReadonlyMap<string, readonly string[]> {
  const t = useT();
  const reads = useQueries({
    queries: records.map((record) =>
      recordAssignmentsQuery(record.type, record.id),
    ),
  });
  const named = new Map<string, string[]>();
  for (const read of reads) {
    for (const row of read.data ?? []) {
      if (row.subject_kind !== "user" || row.subject_inactive) {
        continue;
      }
      const role = shippedLabel(
        {
          label: row.role_label ?? row.role_key,
          label_key: row.role_label_key,
        },
        t,
      );
      const roles = named.get(row.subject_id) ?? [];
      if (!roles.includes(role)) {
        roles.push(role);
      }
      named.set(row.subject_id, roles);
    }
  }
  return named;
}

export function useCreateRecordAssignment(
  recordType: AssignmentRecordType,
  recordId: string,
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateRecordAssignmentRequest"],
    ) => {
      const { data, error, response } = await api.POST(
        "/records/{record_type}/{record_id}/assignments",
        {
          params: { path: { record_type: recordType, record_id: recordId } },
          body,
        },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () =>
      qc.invalidateQueries({
        queryKey: recordAssignmentsKey(recordType, recordId),
      }),
  });
}

export function useUpdateRecordAssignment(
  recordType: AssignmentRecordType,
  recordId: string,
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      id: string;
      body: components["schemas"]["UpdateRecordAssignmentRequest"];
    }) => {
      const { data, error, response } = await api.PATCH("/assignments/{id}", {
        params: { path: { id: args.id } },
        body: args.body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () =>
      qc.invalidateQueries({
        queryKey: recordAssignmentsKey(recordType, recordId),
      }),
  });
}

/** Ending a responsibility keeps it in history; there is no delete. */
export function useArchiveRecordAssignment(
  recordType: AssignmentRecordType,
  recordId: string,
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/assignments/{id}", {
        params: { path: { id } },
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
    },
    onSuccess: () =>
      qc.invalidateQueries({
        queryKey: recordAssignmentsKey(recordType, recordId),
      }),
  });
}

export function useCreateRecordRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateRecordRoleRequest"],
    ) => {
      const { data, error, response } = await api.POST("/record-roles", {
        body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: RECORD_ROLES_KEY }),
  });
}

export function useUpdateRecordRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      id: string;
      body: components["schemas"]["UpdateRecordRoleRequest"];
    }) => {
      const { data, error, response } = await api.PATCH("/record-roles/{id}", {
        params: { path: { id: args.id } },
        body: args.body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: RECORD_ROLES_KEY }),
  });
}
