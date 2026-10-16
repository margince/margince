/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { InstallationConfigurationPage } from "./installation-configuration";
import { jsonResponse, render } from "./settings.testkit";

type Entry = components["schemas"]["ConfigurationEntry"];

const SECRET_TEXT = "sk-live-never-on-screen";

function entry(over: Partial<Entry>): Entry {
  return {
    key: "http.port",
    env: "MARGINCE_HTTP_PORT",
    type: "int",
    doc: "The port the api listens on.",
    roles: ["api"],
    secret: false,
    stored: false,
    value: "8080",
    default: "8080",
    origin: { source: "default", name: "" },
    ...over,
  };
}

const ENTRIES: Entry[] = [
  entry({}),
  entry({
    key: "database.url",
    env: "MARGINCE_DATABASE_URL",
    secret: true,
    value: null,
    default: null,
    origin: { source: "env_file", name: "MARGINCE_DATABASE_URL", file: ".env" },
  }),
  entry({
    key: "worker.concurrency",
    env: "MARGINCE_WORKER_CONCURRENCY",
    roles: ["worker"],
    value: null,
    default: null,
    origin: { source: "unset", name: "" },
  }),
  entry({
    key: "company.name",
    env: "MARGINCE_COMPANY_NAME",
    stored: true,
    value: "Acme",
    default: null,
    origin: { source: "database", name: "" },
  }),
];

function stubConfiguration(status = 200, body: unknown = ENTRIES) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(body, status)),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function rowOf(key: string): HTMLElement {
  const row = screen.getByText(key).closest("tr");
  if (row === null) {
    throw new Error(`no row for ${key}`);
  }
  return row;
}

it("lists every setting with its value, its source and its variable", async () => {
  stubConfiguration();
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  expect(within(rowOf("http.port")).getByText("8080")).toBeInTheDocument();
  expect(within(rowOf("http.port")).getByText("Default value")).toBeVisible();
  expect(
    within(rowOf("http.port")).getByText("MARGINCE_HTTP_PORT"),
  ).toBeVisible();
  expect(within(rowOf("company.name")).getByText("Acme")).toBeVisible();
  expect(
    within(rowOf("company.name")).getByText("Saved in the app"),
  ).toBeVisible();
});

it("names a boot source with its name and its file", async () => {
  stubConfiguration();
  render(<InstallationConfigurationPage />);

  await screen.findByText("database.url");
  expect(
    within(rowOf("database.url")).getByText(
      ".env file: MARGINCE_DATABASE_URL (.env)",
    ),
  ).toBeVisible();
});

it("names a boot source that carries no file by its name alone", async () => {
  stubConfiguration(200, [
    entry({
      origin: { source: "env", name: "MARGINCE_HTTP_PORT" },
    }),
  ]);
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  expect(
    within(rowOf("http.port")).getByText("Environment: MARGINCE_HTTP_PORT"),
  ).toBeVisible();
  expect(document.body.textContent).not.toContain("Change it there");
});

it("shows an empty string as Empty", async () => {
  stubConfiguration(200, [entry({ value: "", default: "" })]);
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  expect(within(rowOf("http.port")).getByText("Empty")).toBeVisible();
});

it("tells a setting with no value and no default from one the api does not read", async () => {
  stubConfiguration(200, [
    entry({ value: null, default: null }),
    entry({
      key: "worker.concurrency",
      value: null,
      default: null,
      origin: { source: "unset", name: "" },
    }),
  ]);
  render(<InstallationConfigurationPage />);

  await screen.findByText("worker.concurrency");
  expect(within(rowOf("http.port")).getByText("No value")).toBeVisible();
  expect(
    within(rowOf("worker.concurrency")).getByText("Not set"),
  ).toBeVisible();
});

it("opens a secret row without showing a value or a default", async () => {
  stubConfiguration(
    200,
    ENTRIES.map((row) =>
      row.secret ? { ...row, value: SECRET_TEXT, default: SECRET_TEXT } : row,
    ),
  );
  const user = userEvent.setup();
  render(<InstallationConfigurationPage />);

  await screen.findByText("database.url");
  await user.click(
    screen.getByRole("button", { name: "Show details for database.url" }),
  );
  expect(
    await screen.findByText(/Type: int/, { selector: "span" }),
  ).toBeVisible();
  expect(screen.queryByText(/Default:/, { selector: "span" })).toBeNull();
  expect(document.body.textContent).not.toContain(SECRET_TEXT);
});

it("marks a secret and draws no value for it", async () => {
  stubConfiguration(
    200,
    ENTRIES.map((row) =>
      row.secret ? { ...row, value: SECRET_TEXT, default: SECRET_TEXT } : row,
    ),
  );
  render(<InstallationConfigurationPage />);

  await screen.findByText("database.url");
  expect(within(rowOf("database.url")).getByText("Secret")).toBeVisible();
  expect(document.body.textContent).not.toContain(SECRET_TEXT);
});

it("shows a setting the api does not read as not set, never as an empty string", async () => {
  stubConfiguration();
  render(<InstallationConfigurationPage />);

  await screen.findByText("worker.concurrency");
  const row = rowOf("worker.concurrency");
  expect(within(row).getByText("Not set")).toBeVisible();
  expect(within(row).queryByText("Empty")).toBeNull();
});

it("opens a row to show its documentation and its default", async () => {
  stubConfiguration();
  const user = userEvent.setup();
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  await user.click(
    screen.getByRole("button", { name: "Show details for http.port" }),
  );
  expect(await screen.findByText("The port the api listens on.")).toBeVisible();
  expect(screen.getByText(/Default: 8080/)).toBeVisible();
});

it("narrows by key and by environment variable", async () => {
  stubConfiguration();
  const user = userEvent.setup();
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  const filter = screen.getByRole("searchbox", {
    name: "Filter by key or environment variable",
  });

  await user.type(filter, "worker.conc");
  expect(screen.getByText("worker.concurrency")).toBeInTheDocument();
  expect(screen.queryByText("http.port")).toBeNull();

  await user.clear(filter);
  await user.type(filter, "margince_company");
  await waitFor(() => expect(screen.queryByText("http.port")).toBeNull());
  expect(screen.getByText("company.name")).toBeInTheDocument();
});

it("says so when nothing matches", async () => {
  stubConfiguration();
  const user = userEvent.setup();
  render(<InstallationConfigurationPage />);

  await screen.findByText("http.port");
  await user.type(
    screen.getByRole("searchbox", {
      name: "Filter by key or environment variable",
    }),
    "zzz-no-such-setting",
  );
  expect(
    await screen.findByText("No setting matches this filter."),
  ).toBeVisible();
});

it("says the page could not be read when the server refuses the caller", async () => {
  stubConfiguration(403, {
    type: "about:blank",
    title: "Forbidden",
    status: 403,
    code: "permission_denied",
    detail: "You may not read the configuration.",
  });
  render(<InstallationConfigurationPage />);

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText(/do not have permission/)).toBeVisible();
  expect(screen.queryByText("http.port")).toBeNull();
});
