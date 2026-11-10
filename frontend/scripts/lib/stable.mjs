// `stable` from src/format/collate.ts, for the plain-Node scripts that cannot
// import the app's TypeScript. stable.test.ts holds the two to one order.

/**
 * Orders two strings by UTF-16 code unit, the same for everyone.
 * @param {string} a
 * @param {string} b
 * @returns {number}
 */
export function stable(a, b) {
  if (a < b) {
    return -1;
  }
  return a > b ? 1 : 0;
}
