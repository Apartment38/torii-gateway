// Development only: emit the checked-in browser script without external tools.
// Requires Node.js with the built-in stripTypeScriptTypes API (22.13+).
import { readFileSync, writeFileSync } from "node:fs";
import { stripTypeScriptTypes } from "node:module";

const presets = JSON.parse(readFileSync(new URL("../internal/intelligence/presets.json", import.meta.url), "utf8"));
const source = `const benchmarkDefinitions = ${JSON.stringify(presets)};\n` + ["benchmarks.ts", "models.ts", "accounting.ts", "activity.ts", "app.ts"].map(file => readFileSync(new URL(`../internal/web/static/${file}`, import.meta.url), "utf8")).join("\n");
const target = new URL("../internal/web/static/app.js", import.meta.url);
const script = "// Generated from benchmarks.ts, models.ts, accounting.ts, activity.ts and app.ts by scripts/build-console.mjs.\n" +
  stripTypeScriptTypes(source, { mode: "strip" })
    .split("\n").map(line => line.trimEnd()).join("\n");
if (process.argv.includes("--check")) {
  if (readFileSync(target, "utf8") !== script) {
    throw new Error("app.js is out of date; run node scripts/build-console.mjs");
  }
} else {
  writeFileSync(target, script);
}
