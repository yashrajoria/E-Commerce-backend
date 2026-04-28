#!/usr/bin/env node
// Simple generator: reads openapi YAML and writes per-tag markdown summaries.

const fs = require("fs");
const yaml = require("js-yaml");
const path = require("path");

if (process.argv.length < 4) {
  console.error("Usage: node generate-api-md.js <openapi.yaml> <outdir>");
  process.exit(2);
}

const openapiPath = process.argv[2];
const outDir = process.argv[3];

const doc = yaml.load(fs.readFileSync(openapiPath, "utf8"));
const tags = {};

for (const p of Object.keys(doc.paths || {})) {
  const methods = doc.paths[p];
  for (const m of Object.keys(methods)) {
    const op = methods[m];
    const opTags = op.tags || ["unlabeled"];
    for (const t of opTags) {
      if (!tags[t]) tags[t] = [];
      tags[t].push({
        method: m.toUpperCase(),
        path: p,
        summary: op.summary || "",
      });
    }
  }
}

if (!fs.existsSync(outDir)) fs.mkdirSync(outDir, { recursive: true });

for (const t of Object.keys(tags)) {
  const file = path.join(outDir, `${t.replace(/\s+/g, "-")}.md`);
  const lines = [];
  lines.push(`# ${t} API`);
  lines.push("");
  lines.push("Endpoints:");
  lines.push("");
  for (const e of tags[t]) {
    lines.push(`- **${e.method}** ${e.path} — ${e.summary}`);
  }
  fs.writeFileSync(file, lines.join("\n"));
}

console.log("Generated", Object.keys(tags).length, "files to", outDir);
