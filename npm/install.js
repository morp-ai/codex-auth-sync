#!/usr/bin/env node

import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { chmod, mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { get } from "node:https";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

import { expectedChecksum, releaseAsset } from "./release.js";

const npmDirectory = path.dirname(fileURLToPath(import.meta.url));
const packageDirectory = path.dirname(npmDirectory);
const packageJSON = JSON.parse(await readFile(path.join(packageDirectory, "package.json"), "utf8"));
const repository = process.env.CODEX_AUTH_SYNC_REPOSITORY || "morp-ai/codex-auth-sync";
const requestedVersion = process.env.CODEX_AUTH_SYNC_VERSION || `v${packageJSON.version}`;
const version = requestedVersion.startsWith("v") ? requestedVersion : `v${requestedVersion}`;
const asset = releaseAsset(process.platform, process.arch);
const baseURL = `https://github.com/${repository}/releases/download/${version}`;
const nativeDirectory = path.join(npmDirectory, "native");
const destination = path.join(nativeDirectory, asset);
const temporary = `${destination}.${process.pid}.tmp`;
const proxyURL = process.env.HTTPS_PROXY || process.env.https_proxy || process.env.HTTP_PROXY || process.env.http_proxy;
const execFileAsync = promisify(execFile);

async function downloadWithCurl(url) {
  try {
    const { stdout } = await execFileAsync(process.platform === "win32" ? "curl.exe" : "curl", [
      "--fail",
      "--silent",
      "--show-error",
      "--location",
      "--max-time",
      "120",
      url
    ], { encoding: null, maxBuffer: 32 * 1024 * 1024 });
    return stdout;
  } catch (error) {
    throw new Error(`curl download failed for ${url}: ${error.message}`);
  }
}

function download(url, redirects = 0) {
  if (redirects > 5) {
    return Promise.reject(new Error(`Too many redirects while downloading ${url}`));
  }
  return new Promise((resolve, reject) => {
    const request = get(url, {
      headers: { "User-Agent": `@morp-ai/codex-auth-sync/${packageJSON.version}` }
    }, (response) => {
      const location = response.headers.location;
      if (response.statusCode >= 300 && response.statusCode < 400 && location) {
        response.resume();
        resolve(download(new URL(location, url).toString(), redirects + 1));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`Download failed with HTTP ${response.statusCode}: ${url}`));
        return;
      }
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => resolve(Buffer.concat(chunks)));
      response.on("error", reject);
    });
    request.setTimeout(120000, () => request.destroy(new Error(`Download timed out: ${url}`)));
    request.on("error", reject);
  });
}

function downloadReleaseFile(url) {
  if (proxyURL) {
    return downloadWithCurl(url);
  }
  return download(url).catch(() => downloadWithCurl(url));
}

try {
  console.log(`Downloading ${asset} from codex-auth-sync ${version}`);
  const [checksumData, binary] = await Promise.all([
    downloadReleaseFile(`${baseURL}/checksums.txt`),
    downloadReleaseFile(`${baseURL}/${asset}`)
  ]);
  const expected = expectedChecksum(checksumData.toString("utf8"), asset);
  const actual = createHash("sha256").update(binary).digest("hex");
  if (actual !== expected) {
    throw new Error(`Checksum verification failed for ${asset}.`);
  }
  await mkdir(nativeDirectory, { recursive: true, mode: 0o755 });
  await writeFile(temporary, binary, { mode: 0o755 });
  await rm(destination, { force: true });
  await rename(temporary, destination);
  if (process.platform !== "win32") {
    await chmod(destination, 0o755);
  }
  console.log(`Installed and verified ${asset}`);
} catch (error) {
  await rm(temporary, { force: true }).catch(() => {});
  console.error(`Error: ${error.message}`);
  process.exitCode = 1;
}
