// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import type { Handler } from "./server";

// The mailbox-privacy surfaces live in the page's state, so a spec that flips
// a setting or overrules a sender reads back what it wrote.

// The request is flat and the record nests the per-read limits, so each wire
// field is filed where a read-back finds it.
export const captureSettings: Handler = (
  { route, method, json },
  { captureSettings: settings },
) => {
  if (method === "PATCH") {
    const asked: components["schemas"]["UpdateCaptureSettingsRequest"] = route
      .request()
      .postDataJSON();
    const {
      site_read_max_pages,
      site_read_max_mib,
      site_read_wall_seconds,
      ...flat
    } = asked;
    Object.assign(settings, flat);
    settings.site_read = {
      max_pages: site_read_max_pages ?? settings.site_read.max_pages,
      max_mib: site_read_max_mib ?? settings.site_read.max_mib,
      wall_seconds: site_read_wall_seconds ?? settings.site_read.wall_seconds,
    };
  }
  return json(settings);
};

export const senders: Handler = ({ json }, { captureSenders }) =>
  json({ data: captureSenders });

export const senderDecision: Handler = (
  { route, path, method, json },
  { captureSenders },
) => {
  const address = decodeURIComponent(
    path.slice("/capture/senders/".length, -"/decision".length),
  );
  const row = captureSenders.find((s) => s.address === address);
  if (!row) {
    return route.fulfill({ status: 404 });
  }
  if (method === "DELETE") {
    row.decision = undefined;
    row.overruled = false;
    return route.fulfill({ status: 204 });
  }
  row.decision = route.request().postDataJSON()?.decision;
  row.overruled_kind = row.kind;
  row.overruled = true;
  return json(row);
};

export const counterpartyHolds: Handler = (
  { route, method, json },
  { counterpartyHolds: holds },
) => {
  if (method !== "POST") {
    return json({ data: holds });
  }
  const body = route.request().postDataJSON();
  const hold = {
    id: `hold-${holds.length + 1}`,
    kind: body.kind,
    value: body.value,
    created_at: "2026-08-01T09:00:00Z",
  };
  holds.push(hold);
  return json(hold, 201);
};

export const releaseHold: Handler = (
  { route, path },
  { counterpartyHolds: holds },
) => {
  const id = path.slice("/capture/counterparty-holds/".length);
  const at = holds.findIndex((h) => h.id === id);
  if (at === -1) {
    return route.fulfill({ status: 404 });
  }
  holds.splice(at, 1);
  return route.fulfill({ status: 204 });
};

// A shared posture is refused unless the workspace allows it, as the server
// refuses it. The option's disabled state and the row's second sentence are
// both about this refusal.
export const mailPosture: Handler = (
  { route, path, json },
  { captureConnections, captureSettings: settings },
) => {
  const provider = path.slice("/connectors/".length, -"/mail-posture".length);
  const conn = captureConnections.find((c) => c.provider === provider);
  const posture = route.request().postDataJSON()?.posture;
  if (!conn) {
    return route.fulfill({ status: 404 });
  }
  if (posture === "shared" && !settings.shared_posture_allowed) {
    return json(
      {
        title: "Unprocessable Entity",
        code: "validation_error",
        detail: "this workspace does not allow a shared mailbox",
      },
      422,
    );
  }
  conn.mail_posture = posture;
  return json(conn);
};

export const connectors: Handler = ({ json }, { captureConnections }) =>
  json({ data: captureConnections });

export const agentTools = {
  data: [
    {
      name: "search_records",
      required_scope: "read",
      tier: "auto_execute",
      egress: false,
    },
    {
      name: "send_email",
      required_scope: "send",
      tier: "confirmation_required",
      egress: true,
    },
  ],
};
