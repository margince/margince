import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import type { AttentionItem } from "./today.queries";

// Where a worklist row goes when the reader presses it.
//
// Every producer on the feed already names the record its item is ABOUT —
// the deal that went quiet, the person who was promised something, the lead a
// task was raised for. The lanes drew that name as text and dropped the
// address, so a rep who read "Salesforce — Einführung, no contact for 83 days"
// had to go and find Salesforce by hand to do anything about it.
//
// `activity` is a subject the feed sends and this app cannot route to: it is a
// timeline entry rather than a record with a page. `isEntityKind` is what tells
// the two apart, so an unroutable subject yields no address instead of a link
// to a screen that does not exist.
export function subjectHref(item: AttentionItem): string | null {
  const subject = item.subject;
  if (!subject || !isEntityKind(subject.type)) {
    return null;
  }
  return routeHash(ENTITY[subject.type].route(subject.id));
}
