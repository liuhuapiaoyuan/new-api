/**
 * Bun: `bun run test/s3.upload.ts`
 * 在项目根 `.env` 设置 `BEARER_TOKEN`；可选 `S3_DIR`（默认 `test-gemini`）校验返回 URL 路径含该目录前缀。
 * PowerShell: `$env:BEARER_TOKEN='sk-...'; bun run test/s3.upload.ts`
 */

declare const Bun: { env: Record<string, string | undefined> };

const endpoint =
  "https://ai-api.qzsyzn.com/v1beta/models/gemini-3.1-flash-image-preview:predict";

function pickInlineUrls(obj: unknown): string[] {
  const out: string[] = [];
  const walk = (v: unknown) => {
    if (v === null || v === undefined) return;
    if (typeof v === "string") return;
    if (Array.isArray(v)) {
      for (const x of v) walk(x);
      return;
    }
    if (typeof v === "object") {
      const o = v as Record<string, unknown>;
      if (typeof o.url === "string" && o.url.startsWith("http")) {
        out.push(o.url);
      }
      if (typeof o.inlineData === "object" && o.inlineData) {
        const id = o.inlineData as Record<string, unknown>;
        if (typeof id.url === "string") out.push(id.url);
      }
      for (const k of Object.keys(o)) {
        if (k === "thoughtSignature") continue;
        walk(o[k]);
      }
    }
  };
  walk(obj);
  return [...new Set(out)];
}

/** 规范化目录前缀，与网关 sanitize 后一致（多段用 / 连接） */
function normalizeDir(d: string): string {
  return d
    .trim()
    .replace(/\\/g, "/")
    .replace(/\.\./g, "")
    .split("/")
    .map((s) => s.trim())
    .filter((s) => s !== "" && s !== ".")
    .join("/");
}

function assertUrlPathContainsDir(urlStr: string, dir: string): void {
  const nd = normalizeDir(dir);
  if (!nd) return;
  let u: URL;
  try {
    u = new URL(urlStr);
  } catch {
    throw new Error(`invalid URL: ${urlStr}`);
  }
  const p = u.pathname;
  const expected = "/" + nd + "/";
  if (!p.includes(expected) && !p.endsWith("/" + nd)) {
    throw new Error(
      `URL path missing S3_DIR prefix "${nd}": ${p} (full: ${urlStr})`
    );
  }
}

function assertNoInlineImageBase64(obj: unknown): void {
  const walk = (v: unknown, path: string) => {
    if (v === null || v === undefined) return;
    if (typeof v !== "object") return;
    if (Array.isArray(v)) {
      v.forEach((x, i) => walk(x, `${path}[${i}]`));
      return;
    }
    const o = v as Record<string, unknown>;
    const id = (o.inlineData ?? o.inline_data) as Record<string, unknown> | undefined;
    if (id && typeof id === "object") {
      const d = id.data;
      if (typeof d === "string" && d.length > 200) {
        throw new Error(
          `inlineData.data still contains base64-like payload (${d.length} chars) at ${path}`
        );
      }
    }
    for (const k of Object.keys(o)) {
      if (k === "thoughtSignature") continue;
      walk(o[k], `${path}.${k}`);
    }
  };
  walk(obj, "$");
}

async function main() {
  const cdnBase = (Bun.env.S3_CDN ?? "https://cdn.kedao.ggss.club/").replace(/\/$/, "");
  const s3Dir = Bun.env.S3_DIR ?? "test-gemini";

  const body: Record<string, unknown> = {
    S3_DIR: s3Dir,
    S3_BUCKET_NAME: Bun.env.S3_BUCKET_NAME ?? "gegeshushu",
    S3_REGION: Bun.env.S3_REGION ?? "cn-south-1",
    S3_ENDPOINT: Bun.env.S3_ENDPOINT ?? "https://s3.cn-south-1.qiniucs.com",
    S3_ACCESS_KEY_ID: Bun.env.S3_ACCESS_KEY_ID ?? "",
    S3_SECRET_ACCESS_KEY: Bun.env.S3_SECRET_ACCESS_KEY ?? "",
    S3_CDN: Bun.env.S3_CDN ?? "https://cdn.kedao.ggss.club/",
    contents: [
      {
        parts: [
          {
            text: "Create a picture of a nano banana dish in a fancy restaurant with a Gemini theme",
          },
        ],
      },
    ],
  };

  const token =  Bun.env.BEARER_TOKEN;
  if (!token) {
    console.error("Set BEARER_TOKEN (e.g. in .env or export)");
    process.exit(1);
  }

  const res = await fetch(endpoint, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    
    body: JSON.stringify(body),
  });

  const text = await res.text();
  console.log("HTTP", res.status, res.statusText);
  console.log("S3_DIR (request):", s3Dir);

  if (!res.ok) {
    console.error(text.slice(0, 2000));
    process.exit(1);
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    console.error("Response is not JSON:", text.slice(0, 500));
    process.exit(1);
  }

  const urls = pickInlineUrls(parsed);
  const cdnHit = urls.some((u) => u.startsWith(cdnBase) || u.includes(new URL(cdnBase).hostname));

  if (!cdnHit && urls.length > 0) {
    console.log("Found URLs (hostname may differ from S3_CDN):", urls);
  }

  if (!cdnHit) {
    const sample = JSON.stringify(parsed, (_k, v) =>
      typeof v === "string" && v.length > 120 ? v.slice(0, 120) + "…" : v
    );
    console.error("FAIL: no https URL under expected CDN base:", cdnBase);
    console.error("Collected URLs:", urls);
    console.error("Body sample:", sample.slice(0, 4000));
    process.exit(1);
  }

  try {
    assertNoInlineImageBase64(parsed);
  } catch (e) {
    console.error("FAIL:", e instanceof Error ? e.message : e);
    process.exit(1);
  }

  if (/"b64_json"\s*:\s*"[A-Za-z0-9+/=]{200,}/.test(text)) {
    console.error("FAIL: response still contains large b64_json");
    process.exit(1);
  }

  for (const u of urls) {
    if (!u.startsWith("http")) continue;
    try {
      assertUrlPathContainsDir(u, s3Dir);
    } catch (e) {
      console.error("FAIL: S3_DIR not reflected in CDN URL path:", e instanceof Error ? e.message : e);
      process.exit(1);
    }
  }

  console.log("PASS: S3/CDN URLs:", urls.filter((u) => u.startsWith("http")));
  console.log("PASS: path contains S3_DIR:", normalizeDir(s3Dir));
  console.log("PASS: no large inlineData.data / b64_json");
  process.exit(0);
}

main();
