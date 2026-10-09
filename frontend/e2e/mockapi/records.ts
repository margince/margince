// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  briefEmpty,
  briefManager,
  briefOmitted,
  briefWithPlan,
} from "../../src/screens/meetingbrief/fixtures";
import {
  anna,
  brandt,
  deals,
  MEETING_ACTIVITY,
  seats,
  seededLead,
  stages,
} from "./fixtures";
import { type Handler, PASS, page } from "./server";

export const versionSkew = {
  title: "Conflict",
  detail: "version skew — reload and retry",
  code: "version_skew",
};

// projectmock.ts answers the project paths and passes on every other one.
export const projects: Handler = async (
  { route, path, method, json },
  { projectState },
) =>
  (await projectState.handle(route, path, method, json)) ? undefined : PASS;

// The patch is remembered, so the screen's refetch after a save reads it.
export const patchDeal: Handler = ({ route, path, json }, { dealPatches }) => {
  const id = path.slice("/deals/".length);
  const base = deals.find((deal) => deal.id === id) ?? deals[0];
  const body = route.request().postDataJSON();
  dealPatches[id] = { ...dealPatches[id], ...body };
  return json({ ...base, ...dealPatches[id] });
};

export const createContact: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  return json(
    {
      ...anna,
      id: "p-new",
      full_name: String(body.full_name),
      title: body.title ?? null,
      emails: body.emails ?? [],
      captured_by: "human:u1",
      source: "manual",
    },
    201,
  );
};

export const newContact = { ...anna, id: "p-new", full_name: "Peter Neu" };

// The contact the search fixture finds through Brandt, so opening that hit
// lands on its own record.
export const jonas = {
  ...anna,
  id: "p-jonas",
  full_name: "Jonas Weiß",
  title: "Fleet manager",
  emails: [],
};

export const createCompany: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  return json(
    {
      ...brandt,
      id: "o-new",
      display_name: String(body.display_name),
      industry: body.industry ?? null,
      captured_by: "human:u1",
      source: "manual",
    },
    201,
  );
};

export const newCompany = {
  ...brandt,
  id: "o-new",
  display_name: "Neue Firma GmbH",
};

function newLead(fields: {
  full_name: string | null;
  email: string | null;
  company_name: string | null;
}) {
  return {
    id: "l-new",
    workspace_id: "w",
    ...fields,
    status: "new",
    score: 0,
    captured_by: "human:u1",
    source: "manual",
    version: 1,
    created_at: "2026-07-06T08:00:00Z",
    updated_at: "2026-07-06T08:00:00Z",
  };
}

export const createLead: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  return json(
    newLead({
      full_name: body.full_name ?? null,
      email: body.email ?? null,
      company_name: body.company_name ?? null,
    }),
    201,
  );
};

export const createdLead = newLead({
  full_name: "Lena Neu",
  email: "lena@neu.example",
  company_name: null,
});

export const createDeal: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  return json(
    {
      ...deals[0],
      id: "d-new",
      name: String(body.name),
      // Both halves echoed as sent. A deal created with no currency comes
      // back with none, as the real server returns an unpriced deal.
      amount_minor: body.amount_minor ?? null,
      currency: body.currency ?? null,
      stage_id: String(body.stage_id),
    },
    201,
  );
};

// The contact page's one composite read. The contract requires `contact`.
export const contact360 = {
  as_of: "2026-06-20T09:00:00Z",
  contact: anna,
  // A booked meeting, so the meetings tab has a "Brief me" to press.
  next_meeting: {
    activity_id: MEETING_ACTIVITY,
    starts_at: "2026-06-24T13:00:00Z",
    subject: "Retrofit-Abstimmung",
    participants: [{ contact_id: "p-anna", full_name: "Anna Weber" }],
  },
  last_inbound_at: "2026-06-18T08:00:00Z",
  last_outbound_at: "2026-06-19T08:00:00Z",
  sections_omitted: [],
};

// The same fixtures the stories and unit tests read, so a story cannot show
// a surface the e2e run never serves.
export const meetingBrief: Handler = ({ json }, { options }) => {
  switch (options.meetingBrief) {
    case "empty":
      return json(briefEmpty);
    case "failed":
      return json(
        {
          type: "about:blank",
          title: "Not found",
          status: 404,
          code: "not_found",
          detail: "That meeting is filed under a different engagement.",
        },
        404,
      );
    case "plan":
      return json(briefWithPlan);
    case "manager":
      return json(briefManager);
    default:
      return json(briefOmitted);
  }
};

export const contactBrief = {
  contact_id: "p-anna",
  generated_at: "2026-06-20T09:00:00Z",
  generated_by: { kind: "agent", agent: "brief" },
  sentences: [],
};

// The company side of the contact 360. `company` and `as_of` are required,
// and every section is readable and empty, as on a fresh account.
export const company360 = {
  as_of: "2026-06-20T09:00:00Z",
  company: brandt,
  sections_omitted: [],
};

const vocabularyRow = {
  sort_order: 0,
  active: true,
  system: true,
  lead_count: 0,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// The administered lead vocabularies, as a fresh installation ships them.
export const leadSources = {
  data: [
    ["manual", "Created manually", "neutral"],
    ["inbound", "Inbound", "high"],
    ["webform", "Web form", "high"],
    ["referral", "Referral", "high"],
    ["import", "Import", "low"],
    ["crawl", "Web research", "low"],
  ].map(([key, label, intent], i) => ({
    id: `src-${key}`,
    key,
    label,
    intent,
    ...vocabularyRow,
    sort_order: (i + 1) * 10,
  })),
  discovered: [],
};

export const disqualifyReasons = {
  data: ["Not a good fit", "Bad timing", "No budget"].map((label, i) => ({
    id: `reason-${i + 1}`,
    label,
    ...vocabularyRow,
    sort_order: (i + 1) * 10,
  })),
};

export const patchLead: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  return json({ ...seededLead, ...body, version: seededLead.version + 1 });
};

export const promotedLead = {
  contact: {
    ...anna,
    id: "p-new",
    full_name: "Jonas Petersen",
    converted_from_lead_id: "l-1",
  },
  merged: false,
  lead_id: "l-1",
};

// Names only the ids asked about. An id the roster does not hold is absent,
// as the real endpoint answers.
export const userNames: Handler = ({ url, json }) => {
  const asked = new Set(url.searchParams.getAll("id"));
  return json({
    data: seats
      .filter((seat) => asked.has(seat.id))
      .map(({ id, display_name }) => ({ id, display_name })),
  });
};

// Two boards, because the deals screen keeps the pipeline in the address.
// With the default alone, a screen that ignores the choice looks the same.
export const pipelines = page([
  {
    id: "pl",
    workspace_id: "w",
    name: "Sales",
    is_default: true,
    position: 0,
    stages,
  },
  {
    id: "pl-partner",
    workspace_id: "w",
    name: "Partner",
    is_default: false,
    position: 1,
    stages: [
      {
        id: "s-partner",
        workspace_id: "w",
        pipeline_id: "pl-partner",
        name: "Referred",
        position: 1,
        semantic: "open",
        win_probability: 30,
      },
    ],
  },
]);

// The board reads the list, so the list shows the writes the detail shows.
export const listDeals: Handler = ({ json }, { dealPatches }) =>
  json(page(deals.map((deal) => ({ ...deal, ...dealPatches[deal.id] }))));

const winEvidenceRequired = {
  title: "Unprocessable",
  status: 422,
  code: "validation_error",
  details: {
    errors: [
      {
        field: "won_without_contract_reason",
        code: "win_evidence_required",
        message:
          "a won deal needs a signed contract with its paper attached, or a reason why there is none",
      },
    ],
  },
};

// The advance is remembered, and held to If-Match like the lead PATCH. A UI
// that forgets the header, or sends a stale version, fails here loudly.
export const advanceDeal: Handler = (
  { route, params, json },
  { dealPatches, projectState },
) => {
  const target = deals.find((deal) => deal.id === params[0]);
  if (!target) {
    return json({ title: "Not Found" }, 404);
  }
  const current = dealPatches[target.id]?.version ?? target.version;
  if (route.request().headers()["if-match"] !== String(current)) {
    return json(versionSkew, 409);
  }
  // A win needs a signed contract (none is seeded) or a stated reason, as
  // deals/deal_advance.go's ensureWinEvidence demands.
  const advance = route.request().postDataJSON();
  if (advance.status === "won" && !advance.won_without_contract_reason) {
    return json(winEvidenceRequired, 422);
  }
  dealPatches[target.id] = {
    ...dealPatches[target.id],
    stage_id: "s4",
    status: "won",
    won_without_contract_reason: advance.won_without_contract_reason ?? null,
    version: current + 1,
  };
  // The win starts the delivery it was sold for, in the same write.
  projectState.startDelivery(
    dealPatches[target.id]?.project_id ?? target.project_id,
  );
  return json({ ...target, ...dealPatches[target.id] });
};

export const getDeal: Handler = ({ path, json }, { dealPatches }) => {
  const base = deals.find((deal) => path.endsWith(deal.id)) ?? deals[0];
  return json({ ...base, ...dealPatches[base.id] });
};
