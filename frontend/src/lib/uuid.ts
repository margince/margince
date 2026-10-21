// A bare record id in RFC 4122 form, any version. A principal such as
// "human:<uuid>" carries a scheme in front and is not one.
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuid(value: string): boolean {
  return UUID.test(value);
}
