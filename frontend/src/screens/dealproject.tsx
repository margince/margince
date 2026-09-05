// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Briefcase } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { ENTITY } from "../app/entity";
import { navigate, routeHash } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FieldGuard } from "../design-system/rbac";
import { Chip } from "../design-system/readings";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { dealRecordKeys } from "./activitykeys";
import { problemMessageOf, throwProblem } from "./common";
import type { CreateField } from "./create";
import { useEntityName } from "./entityref";
import type { Project } from "./projects.form";

// Where a deal meets its project: the picker on the deal form (with the
// inline "new project" it can grow), the chip on the deal page, and the one
// prompt a won deal without a project gets.

type Deal = components["schemas"]["Deal"];

/**
 * The option value that means "make a new project for this deal". Not a
 * uuid, so it can never collide with a real project id, and not empty, which
 * is the "no project" answer.
 */
export const NEW_PROJECT = "__new_project__";

/**
 * The live projects of ONE company, asked of the server, and NOTHING when there
 * is no company to ask about.
 *
 * The difference from `useOpenProjects` is which question is being asked. "What
 * projects exist" has an answer without a company, and the create form pages
 * them and narrows on the anchor itself. "What may THIS deal take" does not: a
 * deal with no company shares a company with no project, so answering with the
 * whole installation offers pairings the server refuses
 * (deal_project_same_org, 422). A caller acting on one deal uses this.
 */
export function useProjectsOfCompany(organizationId?: string): Project[] {
  return useProjectPage(organizationId, Boolean(organizationId));
}

function useProjectPage(
  organizationId: string | undefined,
  enabled: boolean,
): Project[] {
  const projects = useQuery({
    queryKey: ["projects", "open", organizationId ?? "all"],
    queryFn: async () => {
      const { data, error } = await api.GET("/projects", {
        params: {
          query: {
            ...(organizationId ? { organization_id: organizationId } : {}),
            limit: 200,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
    enabled,
    staleTime: 60_000,
  });
  return (projects.data ?? []).filter((project) => project.phase !== "closed");
}

/**
 * The project fields of the deal form: the picker, and the two fields that
 * appear when the reader chooses to start a project here. The picker offers
 * only the projects of the company the SAME form has chosen — the server
 * refuses a project on another company (422) — and stays disabled until a
 * company is named; a project picked under one company is cleared when the
 * company changes.
 */
export function dealProjectFields(
  t: (key: MessageKey) => string,
  projects: readonly Project[],
  // The project this deal already names, kept on the list even when the
  // open-projects page does not reach it (a closed one, say), so the edit
  // form shows the value it has rather than a blank picker whose save would
  // clear it.
  current?: { id: string; label: string },
  // The company `projects` was read FOR. Both forms now read per company — the
  // create form follows the answers the open form publishes — so the list is
  // always the server's answer about one company, and this says which.
  //
  // It is still compared against the form's own answer, and the gap it covers
  // is a render rather than a workaround: the reader changes the company, the
  // new query is in flight, and for that moment the list on hand is the
  // previous company's. Offering it would let a save carry a pairing the server
  // refuses (deal_project_same_org, 422), so the picker holds nothing until the
  // answer for the company on screen arrives.
  narrowedFor?: string,
): CreateField[] {
  return [
    {
      key: "project_id",
      label: "deal.project",
      type: "select",
      optionsFor: (values) => {
        const company = values.organization_id ?? "";
        if (!company) {
          return [];
        }
        // A list the server narrowed is trustworthy only for the company it
        // was narrowed FOR. While a read for a newly chosen company is in
        // flight, this page still answers about the previous one, and the
        // honest answer is none: a list row names only a project's anchor
        // company, so nothing here can tell which of the new company's
        // projects belong.
        const stale = narrowedFor !== company;
        const reachable = stale ? [] : projects;
        // The NAME alone. The key belongs where a reader needs to recognise it
        // — a subject line, the project's own chip — and a picker of one
        // company's projects is already unambiguous without it.
        const options = reachable.map((project) => ({
          value: project.id,
          label: project.name,
        }));
        return [
          // The project the deal already names, kept on the list so the edit
          // form shows the value it has. NOT when the form has moved to
          // another company: that project belongs to the one it left, and
          // keeping it offered is what lets a save carry the old pairing into
          // the new company. Dropping it here is what withdraws it —
          // `submittedValues` blanks a dependent value the options no longer
          // offer.
          ...(current &&
          !stale &&
          !options.some((option) => option.value === current.id)
            ? [{ value: current.id, label: current.label }]
            : []),
          ...options,
          { value: NEW_PROJECT, label: t("deal.projectNew") },
        ];
      },
    },
    {
      key: "new_project_name",
      label: "project.name",
      required: true,
      showWhen: (values) => values.project_id === NEW_PROJECT,
    },
  ];
}

/**
 * The project id the deal body should carry, creating the project first when
 * the form asked for a new one. The new project is born on the deal's
 * company — the one company the two are required to share.
 */
export async function resolveDealProject(
  values: Record<string, string>,
  organizationId: string | null,
  t: (key: MessageKey) => string,
): Promise<string | null> {
  const picked = values.project_id?.trim() ?? "";
  if (picked !== NEW_PROJECT) {
    return picked || null;
  }
  if (!organizationId) {
    throw new Error(t("deal.projectNeedsCompany"));
  }
  const { data, error } = await api.POST("/projects", {
    body: {
      name: values.new_project_name?.trim() ?? "",
      organization_id: organizationId,
      source: "manual",
    },
  });
  if (error) {
    throwProblem(error);
  }
  return data.id;
}

/** The deal's project as a linked chip, or nothing when it has none. */
// A deal shows its project as a CHIP rather than as the ProjectLinks section
// every other record draws, and deliberately so: a deal carries at most one
// project, names it on the form that creates it, and changes it on the form
// that edits it. A section offering "attach" beside a field that already sets
// the same pointer would be two controls writing one column — the duplication
// ProjectLinks exists to end, reintroduced from the other side.
//
// If a deal ever carries several projects, this becomes a ProjectLinks with
// allowsMany and the form field goes.
export function DealProjectChip({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { name } = useEntityName("project", deal.project_id);
  // A withheld project arrives as a null `project_id` with the field named in
  // `masked_fields`, and drawing nothing at all would say this deal has no
  // project — the same reading a deal that genuinely has none gets. The chip
  // slot stays, carrying the mask, so the reader is told there IS a project and
  // that it is not theirs to see. Not a link: there is no id to send them to.
  if (deal.masked_fields?.includes("project_id")) {
    return (
      <Chip icon={Briefcase}>
        <FieldGuard mode="masked" />
      </Chip>
    );
  }
  if (!deal.project_id) {
    return null;
  }
  const href = routeHash(ENTITY.project.route(deal.project_id));
  return (
    <a className="chip chip-link" href={href} data-testid="deal-project">
      <Briefcase size={14} aria-hidden="true" />
      <span>{name ?? t("deal.projectUnnamed")}</span>
    </a>
  );
}

/**
 * StartDeliveryPrompt is the one offer a won deal with no project gets, and
 * only when its company has exactly ONE open project: attach the deal to it
 * and move the project into delivery. Two open projects is a choice the
 * reader makes on the edit form; none is a project to create first.
 */
export function StartDeliveryPrompt({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const queryClient = useQueryClient();
  // The server answers with the projects this deal's company is on — as the
  // customer, a partner or a subcontractor — so there is nothing left to filter
  // here. Comparing organization_id would drop every project the company works
  // as anything but the customer.
  const candidates = useProjectsOfCompany(deal.organization_id ?? undefined);
  const attach = useMutation({
    mutationFn: async (input: {
      dealId: string;
      version: number | undefined;
      project: Project;
      // True once the deal already names the project: the PATCH landed and
      // only the advance is still owed. Two writes, so a failure between
      // them is a state the reader can be left in, and the retry must not
      // attach a second time.
      attached: boolean;
    }) => {
      if (!input.attached) {
        const patched = await api.PATCH("/deals/{id}", {
          params: {
            path: { id: input.dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { project_id: input.project.id },
        });
        if (patched.error) {
          throwProblem(patched.error);
        }
      }
      // A project already in delivery is not moved again: the advance would
      // be a no-op the server may refuse, and the deal is attached either way.
      if (input.project.phase !== "delivering") {
        const advanced = await api.POST("/projects/{id}/advance", {
          params: {
            path: { id: input.project.id },
            ...ifMatch(requireVersion(input.project.version)),
          },
          body: { to_phase: "delivering", reason: null },
        });
        if (advanced.error) {
          throwProblem(advanced.error);
        }
      }
      return input.project;
    },
    onSuccess: (project) => {
      for (const queryKey of dealRecordKeys(deal.id)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
      queryClient.invalidateQueries({ queryKey: ["project", project.id] });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate({ screen: "projects", id: project.id });
    },
    // Whatever landed before the failure is real: re-read both records so
    // the offer below says what is still owed rather than what was owed.
    onError: (_error, input) => {
      for (const queryKey of dealRecordKeys(input.dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({
        queryKey: ["project", input.project.id],
      });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
  });
  // A withheld project arrives as null with `masked_fields` naming it, and
  // is not the same as none; an archived deal takes no writes at all.
  const masked = (deal.masked_fields ?? []).includes("project_id");
  if (
    deal.status !== "won" ||
    masked ||
    deal.archived_at ||
    candidates.length !== 1
  ) {
    return null;
  }
  const project = candidates[0];
  // The retry case: the deal already names this project but the advance
  // failed, so the project is still short of delivery.
  const attached = deal.project_id === project.id;
  if (deal.project_id && !attached) {
    return null;
  }
  if (attached && project.phase === "delivering") {
    return null;
  }
  return (
    <Callout
      tone="info"
      icon={Briefcase}
      title={t("deal.startDeliveryTitle")}
      actions={
        <Button
          small
          variant="primary"
          disabled={attach.isPending}
          data-testid="deal-start-delivery"
          onClick={() =>
            attach.mutate({
              dealId: deal.id,
              version: deal.version,
              project,
              attached,
            })
          }
        >
          {t("deal.startDelivery")}
        </Button>
      }
    >
      {t(attached ? "deal.startDeliveryAttached" : "deal.startDeliveryBody", {
        project: project.name,
      })}
      {attach.isError && (
        <p className="t-caption" role="alert">
          {problemMessageOf(attach.error, t)}
        </p>
      )}
    </Callout>
  );
}
