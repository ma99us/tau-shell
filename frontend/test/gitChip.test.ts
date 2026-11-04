import assert from "node:assert/strict";
import test from "node:test";
import { gitLabel, gitTooltip } from "../src/gitChip.ts";

test("git chip shows dirty and ahead/behind only when known", () => {
  assert.equal(gitLabel({ branch: "main", dirty: false, ahead: 0, behind: 0 }), "main");
  assert.equal(gitLabel({ branch: "main", dirty: true, ahead: 2, behind: 1 }), "main* +2 -1*");
  assert.equal(gitLabel({ branch: "dev", dirty: true, ahead: -1, behind: -1 }), "dev*");
  assert.match(gitTooltip({ branch: "dev", dirty: false, ahead: -1, behind: -1 }), /No upstream/);
});
