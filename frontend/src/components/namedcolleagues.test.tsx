/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { namedColleaguesFirst } from "./assigneepicker";
import {
  assignedRecord,
  useNamedColleagues,
} from "./recordassignments.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function assignment(
  subject_id: string,
  role_label: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `a-${subject_id}-${role_label}`,
    record_type: "company",
    record_id: "co-1",
    subject_kind: "user",
    subject_id,
    subject_name: subject_id,
    subject_inactive: false,
    role_id: "r",
    role_key: role_label,
    role_label,
    role_active: true,
    ...extra,
  };
}

it("names each colleague on the records once, with every role they hold, and leaves out teams and departed members", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      Response.json({
        data: request.url.includes("/deal/")
          ? [assignment("priya", "Delivery lead")]
          : [
              assignment("priya", "Partner management"),
              assignment("team-1", "Support", { subject_kind: "team" }),
              assignment("tom", "Development", { subject_inactive: true }),
            ],
      }),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const records = [
    ...assignedRecord("company", "co-1"),
    ...assignedRecord("deal", "deal-1"),
  ];
  const { result } = renderHook(() => useNamedColleagues(records), {
    wrapper,
  });
  await waitFor(() =>
    expect(result.current.get("priya")).toEqual([
      "Partner management",
      "Delivery lead",
    ]),
  );
  expect([...result.current.keys()]).toEqual(["priya"]);
});

it("lists the named colleagues first, labelled with their roles, and keeps everyone else", () => {
  const ordered = namedColleaguesFirst(
    [
      { value: "anna", label: "Anna" },
      { value: "priya", label: "Priya" },
    ],
    new Map([["priya", ["Partner management"]]]),
  );
  expect(ordered).toEqual([
    { value: "priya", label: "Priya · Partner management" },
    { value: "anna", label: "Anna" },
  ]);
});

it("finds no named colleagues on a contact or a lead", () => {
  expect(assignedRecord("contact", "c-1")).toEqual([]);
  expect(assignedRecord("lead", "l-1")).toEqual([]);
  expect(assignedRecord("project", "p-1")).toEqual([
    { type: "project", id: "p-1" },
  ]);
});
