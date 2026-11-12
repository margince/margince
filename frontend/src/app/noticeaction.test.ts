import { describe, expect, it } from "vitest";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { noticeAction } from "./noticeaction";

describe("manual privacy review destinations", () => {
  it("opens the retained uploaded file through the authorized download endpoint", () => {
    expect(
      noticeAction({ type: "knowledge_document", id: "retained-file" }),
    ).toEqual({
      href: "/v1/knowledge/documents/retained-file",
      label: "corpusAsk.openFile",
    });
  });
  it("opens the recipient's own signature settings", () => {
    expect(noticeAction({ type: "email_signature", id: "signature" })).toEqual({
      href: routeHash(settingsHref("account")),
      label: "settings.signatureEdit",
    });
  });
  it("keeps description review on the matching record", () => {
    expect(noticeAction({ type: "company", id: "company-id" })?.href).toContain(
      "company-id",
    );
  });
  it.each([
    ["legal_entity", "entities"],
    ["signature_template", "email"],
  ])("opens %s reviews on its Company settings tab", (type, tab) => {
    expect(noticeAction({ type, id: "retained-template" })?.href).toBe(
      routeHash(settingsHref("company", tab)),
    );
  });
});

it("opens a retained-review notice at its signature instead of the reader's own settings", () => {
  const action = noticeAction({
    type: "retained_email_signature",
    id: "retained-signature",
  });
  expect(action?.href).toContain("signature_review=retained-signature");
  expect(action?.href).toContain("privacy");
  expect(action?.href).not.toContain("account");
});

it("opens an approval-history review on the audit log narrowed to that approval", () => {
  const action = noticeAction({ type: "approval_history", id: "approval-id" });
  expect(action?.href).toContain("settings/audit");
  expect(action?.href).toContain("entity_type=approval");
  expect(action?.href).toContain("entity_id=approval-id");
});
