import type { Page } from "@playwright/test";
import { HOME_DAY_FRESH_MS } from "../src/lib/workkeys";

export async function refocusStaleHome(page: Page): Promise<number> {
  const advance = HOME_DAY_FRESH_MS + 1;
  await page.evaluate((advance) => {
    // Cache age uses Date.now; performance time must stay native.
    const aged = Date.now() + advance;
    Date.now = () => aged;
    const fields = [
      { name: "visibilityState", hidden: "hidden", visible: "visible" },
      { name: "hidden", hidden: true, visible: false },
    ].map((field) => ({
      ...field,
      original: Object.getOwnPropertyDescriptor(document, field.name),
    }));
    const failures: unknown[] = [];
    try {
      for (const field of fields)
        Object.defineProperty(document, field.name, {
          configurable: true,
          value: field.hidden,
        });
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
      for (const field of fields)
        Object.defineProperty(document, field.name, {
          configurable: true,
          value: field.visible,
        });
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
    } catch (error) {
      failures.push(error);
    }
    for (const field of fields) {
      try {
        if (field.original)
          Object.defineProperty(document, field.name, field.original);
        else if (!Reflect.deleteProperty(document, field.name))
          failures.push(new Error("could not restore document visibility"));
      } catch (error) {
        failures.push(error);
      }
    }
    if (failures.length > 0)
      throw new AggregateError(
        failures,
        "could not emulate and restore document visibility",
      );
  }, advance);
  return advance;
}
