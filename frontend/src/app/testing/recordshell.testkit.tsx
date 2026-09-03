import type { ReactNode } from "react";
import { PageAsideProvider } from "../pageaside";

// The record pages' details pane is claimed through the shell's own memory
// (app/pageaside.tsx): without the provider around it a record screen has no
// pane to open, and every assertion about the cards in it fails for a reason
// that has nothing to do with the record.
//
// The answer is the real provider rather than a stand-in — a test that supplied
// its own column would prove nothing about the one the product draws. It lives
// here rather than being written out in each record suite because six of them
// need exactly this, and a copy per suite is six places to fix when the pane
// changes shape.
//
// Named `.testkit.` rather than `.test.`: the design-system and lint gates skip
// test files, and a helper the real chrome is mounted through answers to the
// app's rules. The suffix is also what tells fe-uat this is not a component
// owing a story — it renders no surface, only the provider a record screen
// needs around it, and a story of it would picture nothing.

/**
 * A record screen with the shell's details pane around it, standing OPEN.
 *
 * Open, because what a suite or a story reaches for is the cards inside the
 * pane, and a reader who has never folded anything finds it closed. Wrap the
 * `ui` a suite renders, inside whatever providers that suite already carries —
 * this adds the pane and nothing else, so a suite keeps its own query client,
 * locale and toast region.
 */
export function RecordShell({ children }: Readonly<{ children: ReactNode }>) {
  return <PageAsideProvider open>{children}</PageAsideProvider>;
}
