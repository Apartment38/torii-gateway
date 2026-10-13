// Validate manual release metadata before any artifact publication.
import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

export function validateRelease(version, notes) {
  const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/.exec(version);
  if (!match || (match[4] || "").split(".").some(id => /^0\d+$/.test(id))) {
    throw new Error("Release tags must be vMAJOR.MINOR.PATCH with optional SemVer prerelease; build metadata is not supported in image tags");
  }
  if (!notes.startsWith(`# ${version}\n`)) throw new Error("Release notes must start with the exact version heading");
  const headings = ["Installation"];
  if (version !== "v0.2.0") headings.push("Database compatibility", "Configuration changes", "Upgrade prerequisites", "Rollback");
  for (const heading of headings) {
    const section = notes.split(`## ${heading}\n`)[1]?.split(/\n## /)[0]?.trim();
    if (!section || /TODO|REPLACE_ME/.test(section)) throw new Error(`Complete the release notes section: ${heading}`);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const version = process.argv[2];
  // Validate the tag before using it in a filesystem path.
  validateRelease(version, `# ${version}\n` + ["Installation", "Database compatibility", "Configuration changes", "Upgrade prerequisites", "Rollback"].map(h => `## ${h}\nValidation\n`).join(""));
  validateRelease(version, readFileSync(new URL(`../docs/releases/${version}.md`, import.meta.url), "utf8"));
}
