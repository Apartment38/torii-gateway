import assert from "node:assert/strict";
import test from "node:test";
import { validateRelease } from "./check-release.mjs";

const notes = version => `# ${version}\n` + ["Installation", "Database compatibility", "Configuration changes", "Upgrade prerequisites", "Rollback"].map(h => `## ${h}\nComplete instructions.\n`).join("");
test("release metadata accepts versions and complete compatibility notes", () => {
  for (const version of ["v0.1.0", "v0.2.0", "v0.2.1-beta.1", "v2.0.0", "v2.1.0-rc.1"]) assert.doesNotThrow(() => validateRelease(version, notes(version)));
});
test("release metadata rejects unsafe tags and incomplete notes", () => {
  for (const version of ["../secret", "v1", "v01.2.3", "v1.2.3-01", "v1.2.3+build", "v1.2.3;echo"]) assert.throws(() => validateRelease(version, notes(version)));
  assert.throws(() => validateRelease("v2.0.0", notes("v1.0.0")));
  assert.throws(() => validateRelease("v2.0.0", notes("v2.0.0").replace("## Rollback", "## Other")));
  assert.throws(() => validateRelease("v2.0.0", notes("v2.0.0").replace("Complete instructions.", "TODO")));
});

test("first release requires installation without upgrade sections", () => {
  const first = "# v0.2.0\n## Installation\nDownload, verify and start the gateway.\n";
  assert.doesNotThrow(() => validateRelease("v0.2.0", first));
  assert.throws(() => validateRelease("v0.2.0", first.replace("## Installation", "## Other")));
  assert.throws(() => validateRelease("v0.2.0", first.replace("Download, verify and start the gateway.", "REPLACE_ME")));
  assert.throws(() => validateRelease("v0.2.1", first.replace("v0.2.0", "v0.2.1")));
});
