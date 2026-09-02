import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useToast } from "../design-system/toast";
import { toMinorUnits } from "../format/minorunits";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { CreateAction, type CreateField } from "./create";
import { mapProjectCreate } from "./projects.form";

// What a rep can START from the company page.
//
// The page could already say everything about an account and do nothing with
// it: opening a deal meant leaving for the pipeline board and re-finding the
// company there, and applying a tag or adding the account to a list had no
// control at all. Each action below is the record's OWN verb — the company is
// the subject, so it is never something the rep has to pick.

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

/**
 * useDealTarget resolves where a deal created from this page should land: the
 * default pipeline and its first OPEN stage.
 *
 * Open only. A deal is born open (INV-CLOSE-PAST), so won and lost are reached
 * through the confirmed advance and must never be a create-time choice — the
 * deal board applies the same rule to its own stage picker.
 */
function useDealTarget() {
  return useQuery({
    queryKey: ["pipelines", "dealTarget"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      const pipeline: Pipeline | undefined =
        data.data.find((candidate) => candidate.is_default) ?? data.data[0];
      const stages: Stage[] = pipeline?.stages ?? [];
      const open = stages.filter((stage) => stage.semantic === "open");
      return { pipeline, open };
    },
  });
}

/**
 * NewDealAction opens a deal on THIS company.
 *
 * The organization is not a field: the rep is standing on the record, so
 * asking them to name it again is asking them to confirm where they already
 * are — and it is the one value they could get wrong.
 */
export function NewDealAction({
  orgId,
  orgName,
  projectId,
}: Readonly<{
  orgId: string;
  orgName: string;
  // The project the new deal is born into, when the verb is offered from a
  // project page. The deal then names the project at birth, which is the only
  // moment the deal and the project are guaranteed to share a company.
  projectId?: string;
}>) {
  const t = useT();
  const target = useDealTarget();
  const pipeline = target.data?.pipeline;
  const open = target.data?.open ?? [];
  // No pipeline, or none of its stages is open, means there is nowhere for a
  // deal to land. A button that can only fail is worse than no button.
  if (!pipeline || open.length === 0) {
    return null;
  }

  const fields: CreateField[] = [
    { key: "name", label: "create.dealName", required: true },
    { key: "amount", label: "create.amount", type: "number", step: "0.01" },
    {
      key: "currency",
      label: "create.currency",
      type: "select",
      required: true,
      options: ["EUR", "USD", "GBP", "CHF"].map((code) => ({
        value: code,
        label: code,
      })),
    },
    {
      key: "stage_id",
      label: "create.stage",
      type: "select",
      required: true,
      options: open.map((stage) => ({ value: stage.id, label: stage.name })),
    },
    { key: "expected_close_date", label: "create.expectedClose", type: "date" },
  ];

  const createDeal = async (values: Record<string, string>) => {
    const amount = values.amount?.trim();
    const { data, error } = await api.POST("/deals", {
      body: {
        name: values.name.trim(),
        pipeline_id: pipeline.id,
        stage_id: values.stage_id,
        // The form takes major units; the wire is minor units.
        //
        // Amount and currency travel together or not at all — the server
        // refuses a half-populated pair (amount_currency_pair), so sending a
        // currency beside an empty amount 422s the field the form presents as
        // optional.
        amount_minor: amount
          ? toMinorUnits(Number(amount), values.currency || "EUR")
          : null,
        currency: amount ? values.currency || "EUR" : null,
        organization_id: orgId,
        project_id: projectId ?? null,
        expected_close_date: values.expected_close_date || null,
        source: "manual",
      },
    });
    if (error) {
      throwProblem(error, t);
    }
    return data;
  };

  return (
    <CreateAction
      label={t("co.deal.new", { name: orgName })}
      // The page that offered the verb is the page that has to show the deal.
      invalidate={projectId ? "project" : "organization360"}
      screen="deals"
      create={createDeal}
      fields={fields}
      aboutId={orgId}
    />
  );
}

/**
 * NewProjectAction opens a project on THIS company.
 *
 * The company is not a field, for the same reason the deal's is not: the rep is
 * standing on the record, and the one value they could get wrong is the one
 * they do not have to give.
 *
 * The owner is not a field either. The server stamps the creator, and the only
 * choices a create form could offer resolve to the creator anyway; reassigning
 * is an edit, on the project's own page.
 */
export function NewProjectAction({
  orgId,
  orgName,
}: Readonly<{ orgId: string; orgName: string }>) {
  const t = useT();
  const fields: CreateField[] = [
    { key: "name", label: "project.name", required: true },
    { key: "description", label: "project.description", type: "textarea" },
    { key: "target_end_date", label: "project.targetEnd", type: "date" },
  ];
  const createProject = async (values: Record<string, string>) => {
    const { data, error } = await api.POST("/projects", {
      // Through the shared mapper, never a body written here: the projects
      // screen creates the same record, and two spellings of one request are
      // how the two come to disagree about an empty date.
      body: mapProjectCreate({ ...values, organization_id: orgId }),
    });
    if (error) {
      throwProblem(error, t);
    }
    return data;
  };
  return (
    <CreateAction
      label={t("co.project.new", { name: orgName })}
      // The record that offered the verb is the record that has to show the
      // project.
      invalidate="organization360"
      screen="projects"
      create={createProject}
      fields={fields}
      aboutId={orgId}
    />
  );
}

/**
 * The one entry of a bounded catalog that carries a typed name, or nothing —
 * and a refusal where the catalog could not answer the question at all.
 *
 * `/tags` is a bounded VOCABULARY rather than a paged list: the
 * server answers up to a governed cap and offers no cursor, so `page.has_more`
 * is an overflow signal a person has to act on and not a page to walk. A caller
 * matching a name against what it was handed therefore cannot read a miss as
 * "no such word" once that flag is set — the word may be one of the ones past
 * the cap. Creating on that guess is the exact failure reading the vocabulary
 * exists to prevent: a near-duplicate word where the name is free, and where it
 * is unique per installation (`uq_tag_name`) a collision with a row nobody can
 * see, which reaches the rep as a 409 about a tag the same cap hides from them.
 *
 * A caller that reads a bounded catalog and ignores the overflow is a shape
 * rather than a one-off, so the decision is spelled once here rather than
 * re-derived at each call site.
 */
function matchInCatalog<Entry>(
  catalog: Readonly<{
    data: readonly Entry[];
    page: Readonly<{ has_more: boolean }>;
  }>,
  carriesTheName: (entry: Entry) => boolean,
  overflow: MessageKey,
  t: (key: MessageKey) => string,
): Entry | undefined {
  const found = catalog.data.find(carriesTheName);
  if (found) {
    return found;
  }
  if (catalog.page.has_more) {
    throwProblem({ title: t(overflow) });
  }
  return undefined;
}

/**
 * resolveTagId turns a typed name into the id of the ONE tag that carries it,
 * creating it when there is none.
 *
 * Two collisions are resolved rather than reported, because neither is
 * something the rep did or can act on:
 *
 *   - the read finds nothing but the create answers 409, meaning somebody
 *     created the same name in between. tag names are unique per workspace
 *     (uq_tag_name on lower(name)), so THEIR row is the asked-for tag;
 *   - the name matches case-insensitively, because "VIP" and "vip" are one
 *     tag to everyone except the database.
 *
 * The catalog is read at call time, never from a cache the component loaded
 * earlier: a first attempt that created the tag and then failed to apply it
 * leaves a tag no snapshot knows about, so a retry would mint a second one.
 */
async function resolveTagId(
  name: string,
  t: ReturnType<typeof useT>,
): Promise<string> {
  const carriesTheName = (tag: { name: string }) =>
    tag.name.trim().toLowerCase() === name.toLowerCase();

  const { data: known, error: readError } = await api.GET("/tags", {
    params: { query: {} },
  });
  if (readError) {
    throwProblem(readError, t);
  }
  const existing = matchInCatalog(known, carriesTheName, "co.tags.overCap", t);
  if (existing) {
    return existing.id;
  }

  const { data, error, response } = await api.POST("/tags", { body: { name } });
  if (response.status !== 409) {
    if (error) {
      throwProblem(error, t);
    }
    return data.id;
  }

  const { data: after, error: afterError } = await api.GET("/tags", {
    params: { query: {} },
  });
  if (afterError) {
    throwProblem(afterError, t);
  }
  const winner = matchInCatalog(after, carriesTheName, "co.tags.overCap", t);
  if (!winner) {
    // The name collided, the catalog is inside its cap, and no readable row
    // carries the name: the collision was about something other than the name
    // this rep typed — an archived row, which the live read does not carry,
    // holds it. The server's own answer is the honest one here; the overflow
    // reading above is not, because there is no cap to blame.
    throwProblem(error, t);
  }
  return winner.id;
}

/**
 * TagAction puts a tag on this company, creating the tag when the name is new.
 *
 * One field, one name typed. Splitting it into "pick an existing tag" and
 * "make a new one" makes the rep answer a question about the workspace's tag
 * table before they can answer the one they actually have — and on a fresh
 * workspace the pick-only version has nothing to offer, so the control
 * disappears exactly when it is first needed.
 */
export function TagAction({ orgId }: Readonly<{ orgId: string }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();

  // `DELETE /tags/{id}/apply` is the contract's own words for this: "Undo for
  // applyTag ... Idempotent". It is the cleanest inverse pair the API has, and
  // one of the few places in this product where an Undo has something real
  // behind it — most destructive verbs here are archives with no restore.
  //
  // It reports its own refusal rather than failing quietly. A reader watched
  // the confirmation go and would otherwise believe the tag came off.
  const takeTagOff = useMutation({
    mutationFn: async ({ tagId }: { tagId: string; name: string }) => {
      const { error } = await api.DELETE("/tags/{id}/apply", {
        params: { path: { id: tagId } },
        body: { entity_type: "organization", entity_id: orgId },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { mark: false, sticky: true });
    },
    onSuccess: (_removed, { name }) => {
      queryClient.invalidateQueries({ queryKey: ["organization360"] });
      toast.show(t("co.tags.removed", { name }));
    },
  });

  const addTag = async (values: Record<string, string>) => {
    const name = values.name.trim();
    const tagId = await resolveTagId(name, t);
    const { data, error, response } = await api.POST("/tags/{id}/apply", {
      params: { path: { id: tagId } },
      body: { entity_type: "organization", entity_id: orgId },
    });
    // Already on this company is the state the rep asked for, so it is not an
    // error to report at them. The server says 409 because the row exists;
    // what the rep wanted was the tag present, and it is.
    //
    // It carries NO Undo, and the distinction is the point: this press did not
    // put the tag there, so taking it off would reverse a decision somebody
    // else made — which is not what the word means.
    if (response.status === 409) {
      toast.show(t("co.tags.alreadyThere", { name }), { mark: false });
      return { id: orgId };
    }
    if (error) {
      throwProblem(error, t);
    }
    toast.show(t("co.tags.applied", { name }), {
      action: {
        label: t("common.undo"),
        onAct: () => takeTagOff.mutate({ tagId, name }),
      },
    });
    return { id: data.id };
  };

  return (
    <CreateAction
      label={t("co.tags.apply")}
      invalidate="organization360"
      screen="companies"
      stay
      create={addTag}
      fields={[{ key: "name", label: "co.tags.pick", required: true }]}
    />
  );
}
