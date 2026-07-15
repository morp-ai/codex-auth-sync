const MAX_BODY_BYTES = 4 * 1024 * 1024;
const VALID_KINDS = new Set([
  "account_upsert",
  "account_delete",
  "alias_set",
  "account_status",
  "active_set",
  "settings_set"
]);

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname === "/v1/health" && request.method === "GET") {
      return json({ ok: true, service: "codex-auth-sync", protocol: 1 });
    }
    if (url.pathname !== "/v1/events") {
      return json({ error: "not found" }, 404);
    }
    if (!(await authorized(request, env.SYNC_API_TOKEN || ""))) {
      return json({ error: "unauthorized" }, 401);
    }
    if (request.method === "GET") {
      return listEvents(url, env.DB);
    }
    if (request.method === "POST") {
      return appendEvent(request, env.DB);
    }
    return json({ error: "method not allowed" }, 405);
  }
};

async function listEvents(url, db) {
  const clusterID = (url.searchParams.get("cluster_id") || "").trim();
  if (!clusterID) return json({ error: "cluster_id is required" }, 400);
  const after = parseInteger(url.searchParams.get("after"), 0, 0, Number.MAX_SAFE_INTEGER);
  const limit = parseInteger(url.searchParams.get("limit"), 500, 1, 500);
  if (after === null || limit === null) {
    return json({ error: "invalid after or limit value" }, 400);
  }
  const rows = await db.prepare(
    "SELECT revision, event_id, cluster_id, node_id, kind, entity_id, ciphertext, created_at " +
    "FROM sync_events WHERE cluster_id = ?1 AND revision > ?2 ORDER BY revision ASC LIMIT ?3"
  ).bind(clusterID, after, limit).all();
  const latest = await db.prepare("SELECT COALESCE(MAX(revision), 0) AS latest FROM sync_events").first();
  return json({ events: rows.results || [], latest_revision: Number(latest.latest || 0) });
}

async function appendEvent(request, db) {
  const contentLength = Number(request.headers.get("content-length") || 0);
  if (contentLength > MAX_BODY_BYTES) return json({ error: "request is too large" }, 413);
  const text = await request.text();
  if (new TextEncoder().encode(text).length > MAX_BODY_BYTES) {
    return json({ error: "request is too large" }, 413);
  }
  let event;
  try {
    event = JSON.parse(text);
  } catch {
    return json({ error: "invalid event JSON" }, 400);
  }
  const validationError = validateEvent(event);
  if (validationError) return json({ error: validationError }, 400);
  const createdAt = new Date().toISOString();
  await db.prepare(
    "INSERT OR IGNORE INTO sync_events " +
    "(event_id, cluster_id, node_id, kind, entity_id, ciphertext, created_at) " +
    "VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)"
  ).bind(
    event.event_id,
    event.cluster_id,
    event.node_id,
    event.kind,
    event.entity_id,
    event.ciphertext,
    createdAt
  ).run();
  const saved = await db.prepare(
    "SELECT revision, event_id, cluster_id, node_id, kind, entity_id, ciphertext, created_at " +
    "FROM sync_events WHERE event_id = ?1"
  ).bind(event.event_id).first();
  return json(saved, 201);
}

function validateEvent(event) {
  if (!event || typeof event !== "object") return "event must be an object";
  if (!/^[0-9a-f]{32}$/.test(event.event_id || "")) return "event_id must be hexadecimal";
  if (typeof event.cluster_id !== "string" || event.cluster_id.length < 1 || event.cluster_id.length > 128) return "invalid cluster_id";
  if (typeof event.node_id !== "string" || event.node_id.length < 1 || event.node_id.length > 128) return "invalid node_id";
  if (!VALID_KINDS.has(event.kind)) return "unsupported event kind";
  if (!/^[0-9a-f]{64}$/.test(event.entity_id || "")) return "entity_id must be hexadecimal";
  if (typeof event.ciphertext !== "string" || event.ciphertext.length < 1 || event.ciphertext.length > MAX_BODY_BYTES - 1024) return "invalid ciphertext";
  return "";
}

function parseInteger(value, fallback, minimum, maximum) {
  if (value === null || value === "") return fallback;
  if (!/^[0-9]+$/.test(value)) return null;
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < minimum || parsed > maximum) return null;
  return parsed;
}

async function authorized(request, expected) {
  const value = request.headers.get("authorization") || "";
  const provided = value.startsWith("Bearer ") ? value.slice(7).trim() : "";
  const [a, b] = await Promise.all([digest(provided), digest(expected)]);
  if (a.length !== b.length) return false;
  let difference = 0;
  for (let i = 0; i < a.length; i += 1) difference |= a[i] ^ b[i];
  return difference === 0 && provided.length > 0;
}

async function digest(value) {
  const data = new TextEncoder().encode(value);
  return new Uint8Array(await crypto.subtle.digest("SHA-256", data));
}

function json(value, status = 200) {
  return new Response(JSON.stringify(value) + "\n", {
    status,
    headers: {
      "content-type": "application/json",
      "cache-control": "no-store",
      "x-content-type-options": "nosniff"
    }
  });
}
