#!/usr/bin/env node

import { access } from "node:fs/promises";
import { constants } from "node:fs";
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { releaseAsset } from "./release.js";

const directory = path.dirname(fileURLToPath(import.meta.url));
const binary = path.join(directory, "native", releaseAsset(process.platform, process.arch));
try {
  await access(binary, process.platform === "win32" ? constants.F_OK : constants.X_OK);
} catch (error) {
  await import("./install.js");
  try {
    await access(binary, process.platform === "win32" ? constants.F_OK : constants.X_OK);
  } catch {
    console.error(`Error: native codex-auth-sync binary is unavailable: ${error.message}`);
    console.error("Reinstall with: npm install -g git+https://github.com/morp-ai/codex-auth-sync.git");
    process.exit(1);
  }
}

const child = spawn(binary, process.argv.slice(2), { stdio: "inherit", windowsHide: false });

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    try {
      child.kill(signal);
    } catch {
      child.kill();
    }
  });
}

child.once("error", (error) => {
  console.error(`Error: failed to start native codex-auth-sync: ${error.message}`);
  process.exitCode = 1;
});

child.once("exit", (code, signal) => {
  if (signal && process.platform !== "win32") {
    process.kill(process.pid, signal);
    return;
  }
  process.exitCode = code ?? 1;
});
