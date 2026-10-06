/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordView } from "./recordview";

// How TimelineText draws a body by kind: a note as markdown whose links show
// their real address, a mail as plain text with its bare addresses linked.

afterEach(cleanup);

const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

describe("TimelineText's links in a mail", () => {
  it("takes a balanced parenthesis and leaves the prose's closing one", () => {
    render(
      <RecordView
        identity="r-1"
        name="Acme"
        zone="UTC"
        timeline={[
          {
            id: "a-mail",
            kind: "email",
            title: "Passwort",
            atIso: "2026-07-01T00:00:00Z",
            provenance: { kind: "human" as const, self: true },
            body: "Neu setzen (über https://kunde.de/reset/(abc)) oder HTTPS://kunde.de/hilfe.",
          },
        ]}
      />,
    );
    expect(
      screen.getAllByRole("link").map((a) => a.getAttribute("href")),
    ).toEqual(["https://kunde.de/reset/(abc)", "HTTPS://kunde.de/hilfe"]);
  });
});

describe("TimelineText on a note", () => {
  const row = (kind: "note" | "email", body: string) => (
    <RecordView
      identity="r-1"
      name="Acme"
      zone="UTC"
      timeline={[
        {
          id: `a-${kind}`,
          kind,
          title: "Gespräch",
          atIso: "2026-07-01T00:00:00Z",
          provenance: { kind: "human" as const, self: true },
          body,
        },
      ]}
    />
  );

  it("draws a note's headings, lists and emphasis", () => {
    const { container } = render(
      row(
        "note",
        "## Summary\n\n- point one\n- point two\n\n**Next:** call back",
      ),
    );
    expect(screen.getByRole("heading", { name: "Summary" })).toBeTruthy();
    const items = [...container.querySelectorAll(".tl-text li")].map(
      (li) => li.textContent,
    );
    expect(items).toEqual(["point one", "point two"]);
    expect(screen.getByText("Next:").tagName).toBe("STRONG");
    expect(screen.queryByText(/##|\*\*/)).toBeNull();
  });

  it("shows raw HTML in a note as the characters it is", () => {
    const { container } = render(
      row("note", '<script>alert(1)</script> <img src=x onerror="alert(2)">'),
    );
    expect(container.querySelector(".tl-text script, .tl-text img")).toBeNull();
    expect(screen.getByText(/<script>alert\(1\)<\/script>/)).toBeTruthy();
  });

  it("keeps a plain note's line breaks and links its bare address", () => {
    const { container } = render(
      row("note", "Rückruf am Dienstag\nAgenda: https://kunde.de/plan"),
    );
    const body = container.querySelector(".tl-text-clamp");
    expect(body?.textContent).toBe(
      "Rückruf am Dienstag\nAgenda: https://kunde.de/plan",
    );
    expect(
      screen.getByRole("link", { name: "https://kunde.de/plan" }),
    ).toBeTruthy();
    expect(
      container.querySelector(".tl-text h1, .tl-text h2, .tl-text ul"),
    ).toBeNull();
  });

  it("shows a note link's real address, never only its label", () => {
    render(row("note", "[Your bank](https://evil.example/login)"));
    expect(screen.getByRole("link").textContent).toBe(
      "https://evil.example/login",
    );
  });

  it("leaves a mail body's markdown characters as text", () => {
    render(row("email", "## Summary\n\n- point one"));
    expect(screen.queryByRole("heading", { name: "Summary" })).toBeNull();
    expect(screen.getByText(/## Summary/)).toBeTruthy();
  });
});
