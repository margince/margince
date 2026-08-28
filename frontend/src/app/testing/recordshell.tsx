import type { ReactNode } from "react";
import { PageAsideProvider, PageAsideRegion } from "../pageaside";

// The record pages' context column is the SHELL's, not the record's: a record
// fills it through a portal (app/pageaside.tsx). A suite that mounts a record
// screen on its own therefore has nowhere for those cards to land, and every
// assertion about them fails for a reason that has nothing to do with the
// record.
//
// The answer is the real region rather than a stand-in — a test that supplied
// its own column would prove nothing about the one the product draws. It lives
// here rather than being written out in each record suite because six of them
// need exactly this pair, and a copy per suite is six places to fix when the
// shell's column changes shape.
//
// It is NOT a *.test.* file, on purpose, for the same reason
// src/app/testing/shellharness.tsx is not: the design-system and lint gates skip
// test files, and a helper the real chrome is mounted through answers to the
// app's rules.

/**
 * A record screen with the shell's context column around it.
 *
 * Wrap the `ui` a suite renders, inside whatever providers that suite already
 * carries — this adds the column and nothing else, so a suite keeps its own
 * query client, locale and toast region.
 */
export function RecordShell({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <PageAsideProvider>
      {children}
      <PageAsideRegion />
    </PageAsideProvider>
  );
}
