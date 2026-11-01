import { test } from "node:test";
import assert from "node:assert/strict";
import { duplicateAndLaunch } from "../src/sessionActions.ts";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

test("clone action remains pending until launch finishes", async () => {
  const launching = deferred<void>();
  const enteredLaunch = deferred<void>();
  const session = { id: "copy" };
  const action = duplicateAndLaunch(
    async () => session,
    async () => undefined,
    async (copy) => {
      assert.equal(copy, session);
      enteredLaunch.resolve();
      await launching.promise;
    },
  );
  await enteredLaunch.promise;
  let settled = false;
  void action.then(() => { settled = true; });
  await Promise.resolve();
  assert.equal(settled, false);
  launching.resolve();
  assert.equal(await action, session);
});

test("clone action propagates failure and can be retried", async () => {
  const failure = new Error("launch failed");
  await assert.rejects(duplicateAndLaunch(async () => ({ id: "copy" }), async () => undefined, async () => { throw failure; }), failure);
  const result = await duplicateAndLaunch(async () => ({ id: "copy" }), async () => undefined, async () => undefined);
  assert.equal(result.id, "copy");
});