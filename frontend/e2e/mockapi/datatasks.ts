// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Handler } from "./server";

export const list: Handler = ({ json }, state) => {
  if (state.options.dataTasks === "withheld") {
    return json({ title: "Forbidden", status: 403 }, 403);
  }
  if (state.options.dataTasks === "unavailable") {
    return json({ title: "Progress unavailable", status: 503 }, 503);
  }
  return json({ data: state.dataTasks });
};

export const control: Handler = ({ json, route }, state) => {
  if (state.options.dataTasks === "withheld") {
    return json({ title: "Forbidden", status: 403 }, 403);
  }
  const body: unknown = route.request().postDataJSON();
  if (
    typeof body !== "object" ||
    body === null ||
    !("task_id" in body) ||
    !("step" in body) ||
    !("action" in body) ||
    !("version" in body)
  ) {
    return json({ title: "Invalid control", status: 422 }, 422);
  }
  const task = state.dataTasks.find(
    (row) => row.task_id === body.task_id && row.step === body.step,
  );
  if (!task) return json({ title: "Not found", status: 404 }, 404);
  if (task.version !== body.version) {
    return json({ title: "Refresh progress", status: 409 }, 409);
  }
  if (
    body.action === "pause" &&
    (task.status === "pending" || task.status === "running")
  ) {
    task.status = "paused";
  } else if (body.action === "resume" && task.status === "paused") {
    task.status = "pending";
  } else if (body.action === "retry" && task.status === "failed") {
    task.status = "pending";
    task.consecutive_failures = 0;
  } else if (body.action === "estimate") {
    task.estimate = 800;
    task.estimate_capped = false;
    task.estimated_at = "2026-01-15T09:31:00Z";
  } else return json({ title: "Refresh progress", status: 409 }, 409);
  task.version += 1;
  return route.fulfill({ status: 204 });
};
