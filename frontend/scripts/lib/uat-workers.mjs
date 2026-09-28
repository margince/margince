// How many pages the render gate (fe-uat.mjs) captures on at once, and the
// queue they drain. Kept out of the gate so the rules can be asserted.

// Half the cores, at most four: the lane shares its runner with other lanes.
const DEFAULT_CEILING = 4;

export function requestedWorkers(override, cpus) {
  if (override === undefined) {
    return Math.max(1, Math.min(DEFAULT_CEILING, Math.floor(cpus / 2)));
  }
  if (!/^[1-9]\d*$/.test(override)) {
    throw new Error(
      `FE_UAT_WORKERS must be a positive integer, got "${override}"`,
    );
  }
  return Number(override);
}

export function workersFor(requested, stories) {
  return Math.min(requested, stories);
}

// Results keep the items' order whatever order the lanes finish in; a task that
// throws yields recover(item, error) and its lane carries on.
export async function drainInOrder(items, lanes, task, recover) {
  const results = new Array(items.length);
  let next = 0;
  const lane = async (resource) => {
    while (next < items.length) {
      const index = next++;
      try {
        results[index] = await task(items[index], resource);
      } catch (error) {
        results[index] = recover(items[index], error);
      }
    }
  };
  await Promise.all(lanes.map(lane));
  return results;
}

// One crash then fails the item it happened on, not every item its lane draws
// after it.
export function renewingBroken(task, { isBroken, renew }) {
  return async (item, lane) => {
    if (isBroken(lane)) await renew(lane);
    return await task(item, lane);
  };
}
