// Development only: emit the checked-in browser script without external tools.
// Requires Node.js with the built-in stripTypeScriptTypes API (22.13+).
import { readFileSync, writeFileSync } from "node:fs";
import { stripTypeScriptTypes } from "node:module";

const source = new URL("../internal/web/static/app.ts", import.meta.url);
const target = new URL("../internal/web/static/app.js", import.meta.url);
const script = "// Generated from app.ts by scripts/build-console.mjs.\n" +
  stripTypeScriptTypes(readFileSync(source, "utf8"), { mode: "strip" })
    .split("\n").map(line => line.trimEnd()).join("\n");
if (process.argv.includes("--check")) {
  if (readFileSync(target, "utf8") !== script) {
    throw new Error("app.js is out of date; run node scripts/build-console.mjs");
  }
} else {
  writeFileSync(target, script);
}
