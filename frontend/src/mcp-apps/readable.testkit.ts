import { expect } from "vitest";

const RECORD_ID =
  /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;
const COLUMN_NAME = /\b[a-z]+(_[a-z]+)+\b/;

/** expectReadable holds a drawn card to words a reader can use: a heading, a
 *  line under it, and no record id or column name anywhere. */
export function expectReadable(root: HTMLElement): void {
  expect(root.querySelector("h1")?.textContent).toBeTruthy();
  expect(root.querySelector(".panel-body")?.textContent).toBeTruthy();
  expect(root.textContent).not.toMatch(RECORD_ID);
  expect(root.textContent).not.toMatch(COLUMN_NAME);
}
