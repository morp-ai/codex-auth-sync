const platforms = {
  linux: "linux",
  darwin: "darwin",
  win32: "windows"
};

const architectures = {
  x64: "amd64",
  arm64: "arm64"
};

export function releaseAsset(platform, architecture) {
  const os = platforms[platform];
  if (!os) {
    throw new Error(`Unsupported operating system: ${platform}`);
  }
  const arch = architectures[architecture];
  if (!arch) {
    throw new Error(`Unsupported CPU architecture: ${architecture}`);
  }
  const suffix = os === "windows" ? ".exe" : "";
  return `codex-auth-sync-${os}-${arch}${suffix}`;
}

export function expectedChecksum(contents, asset) {
  for (const line of contents.split(/\r?\n/u)) {
    const fields = line.trim().split(/\s+/u);
    if (fields.length === 2 && fields[1] === asset && /^[a-fA-F0-9]{64}$/u.test(fields[0])) {
      return fields[0].toLowerCase();
    }
  }
  throw new Error(`The release checksum file does not contain ${asset}.`);
}
