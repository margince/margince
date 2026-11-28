import { describe, expect, it } from "vitest";
import { visibleWords } from "../testing/record-noun.testkit";
import { en } from "./en";

// The company record is called a company, and English once called it an
// account. The word also names a mail, user, channel or provider account, so a
// sweep of every value cannot tell the two apart. The key can. These keys, and
// the keys under these prefixes, use "account" for the company record. Their
// English value must not.
const COMPANY_SENSE_ACCOUNT_KEYS = [
  "analytics.sectionAccounts",
  "analytics.accounts.",
  "deal.aside.account",
  "dealSuggestion.accountOwner",
  "linkedinReach.account",
  "co.contacts.map.account",
  "rollup.accounts",
  "reporting.prepared.accounts",
  "reporting.account_cohort",
  "agent.activity.accountScan",
  "jobs.kind.account_scan",
  "aiTask.name.account_scan",
  "worklist.because.account_termination",
];

describe("en names the company record a company", () => {
  const catalog: Record<string, string> = en;
  const underCompanySense = (key: string) =>
    COMPANY_SENSE_ACCOUNT_KEYS.some((prefix) => key.startsWith(prefix));

  it("no key that names the company with the old word says account", () => {
    const stale = Object.keys(en).filter(
      (key) =>
        underCompanySense(key) &&
        /\baccounts?\b/i.test(visibleWords(catalog[key])),
    );
    expect(stale).toEqual([]);
  });

  it("every listed key or prefix still names a key", () => {
    const spent = COMPANY_SENSE_ACCOUNT_KEYS.filter(
      (prefix) => !Object.keys(en).some((key) => key.startsWith(prefix)),
    );
    expect(spent).toEqual([]);
  });
});
