// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Handler, PASS } from "./server";
import { worklist } from "./work";

// The primary number is the second, so a Call that dials the first one fails.
// Its own contact, so the specs that read Anna's page do not move.

export const CALLED_CONTACT = "p-kai";
export const PRIMARY_PHONE = "+491705550101";

export const calledContact = {
  id: CALLED_CONTACT,
  workspace_id: "w",
  full_name: "Kai Brenner",
  title: "Fleet manager",
  emails: [
    { id: "e-kai", email: "kai.brenner@brandt.example", is_primary: true },
  ],
  primary_email: "kai.brenner@brandt.example",
  phones: [
    {
      id: "ph-kai-desk",
      phone: "+49301234567",
      phone_type: "work",
      is_primary: false,
    },
    {
      id: "ph-kai-mobile",
      phone: PRIMARY_PHONE,
      phone_type: "mobile",
      is_primary: true,
    },
  ],
  writable: true,
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

const onContact = [{ entity_type: "contact", entity_id: CALLED_CONTACT }];

function activity(fields: {
  id: string;
  kind: string;
  subject: string;
  occurred_at: string;
}) {
  return {
    ...fields,
    workspace_id: "w",
    body: null,
    is_done: false,
    content_state: "available",
    links: onContact,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: fields.occurred_at,
    updated_at: fields.occurred_at,
  };
}

export const loggedCall = activity({
  id: "01a05500-0000-7000-8000-0000000000c1",
  kind: "call",
  subject: "Rückruf zur Flottenumrüstung",
  occurred_at: "2026-06-18T09:00:00Z",
});

export const loggedNote = activity({
  id: "01a05500-0000-7000-8000-0000000000c2",
  kind: "note",
  subject: "Notiz zum Rahmenvertrag",
  occurred_at: "2026-06-12T09:00:00Z",
});

export const taskFromCall = {
  ...activity({
    id: "01a05500-0000-7000-8000-0000000000c3",
    kind: "task",
    subject: "Angebot nach dem Anruf schicken",
    occurred_at: "2026-06-18T09:30:00Z",
  }),
  due_at: "2026-07-06T13:00:00Z",
  source_activity_id: loggedCall.id,
};

export const typedTask = {
  ...activity({
    id: "01a05500-0000-7000-8000-0000000000c4",
    kind: "task",
    subject: "Referenzliste vorbereiten",
    occurred_at: "2026-06-19T09:30:00Z",
  }),
  due_at: "2026-07-06T13:00:00Z",
};

export const calledContact360 = {
  as_of: "2026-06-20T09:00:00Z",
  contact: calledContact,
  activities: {
    data: [loggedCall, loggedNote],
    page: { next_cursor: null, has_more: false },
  },
  sections_omitted: [],
};

const readable = [loggedCall, loggedNote, taskFromCall, typedTask];

// The other activity ids keep the answer the rows below this one give them.
export const activityById: Handler = ({ params, json }) => {
  const row = readable.find((entry) => entry.id === params[0]);
  return row ? json(row) : Promise.resolve(PASS);
};

function taskRow(task: typeof typedTask) {
  return {
    id: task.id,
    source: "task",
    category: "tasks",
    level: 3,
    consequence: "task_slips",
    title: task.subject,
    due_at: task.due_at,
    because: [{ kind: "due_today" }],
    actions: [],
    dispositions: ["snooze", "not_mine"],
  };
}

export const callTasksWorklist = {
  ...worklist,
  summary: { urgent: 0, due: 2, lower_priority: 0, total: 2 },
  reach: [{ source: "task", considered: 2, shown: 2, more_available: false }],
  counts: [
    { category: "tasks", considered: 2, shown: 2, more_available: false },
  ],
  readings: { ...worklist.readings, review: 0 },
  queue: [taskRow(taskFromCall), taskRow(typedTask)],
};
