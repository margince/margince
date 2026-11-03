import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { SERVICE_ICON, SERVICE_WORDS } from "./connect-services";

// Two screens show the same service row. One also says when it last synced,
// the other says what the step is for. They word the row alike because one
// place names it.
describe("one place names a service", () => {
  it("names and describes every service the icons cover", () => {
    for (const service of Object.keys(
      SERVICE_ICON,
    ) as (keyof typeof SERVICE_ICON)[]) {
      expect(SERVICE_WORDS[service]?.name).toBeTruthy();
      expect(SERVICE_WORDS[service]?.desc).toBeTruthy();
    }
  });

  // Keys rather than words, so a renamed catalog entry is a compile error on
  // the type and a failure here if the catalog drops it.
  it("names keys the catalog carries", () => {
    for (const words of Object.values(SERVICE_WORDS)) {
      expect(en).toHaveProperty(words.name);
      expect(en).toHaveProperty(words.desc);
    }
  });
});
