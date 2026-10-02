import assert from "node:assert/strict";
import test from "node:test";
import { validateRelease } from "./check-release.mjs";

const notes = version => `# ${version}\n` + ["Database compatibility", "Configuration changes", "Upgrade prerequisites", "Rollback"].map(h => `## ${h}\nComplete instructions.\n`).join("");
test("release metadata accepts versions and complete compatibility notes", () => {
  for (const version of ["v2.0.0", "v2.1.0-rc.1"]) assert.doesNotThrow(() => validateRelease(version, notes(version)));
});
test("release metadata rejects unsafe tags and incomplete notes", () => {
  for (const version of ["../secret", "v1", "v01.2.3", "v1.2.3-01", "v1.2.3+build", "v1.2.3;echo"]) assert.throws(() => validateRelease(version, notes(version)));
  assert.throws(() => validateRelease("v2.0.0", notes("v1.0.0")));
  assert.throws(() => validateRelease("v2.0.0", notes("v2.0.0").replace("## Rollback", "## Other")));
  assert.throws(() => validateRelease("v2.0.0", notes("v2.0.0").replace("Complete instructions.", "TODO")));
});
