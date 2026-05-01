// worker/scripts/e2e-server.ts
import { createServer } from "node:http";
import { Readable } from "node:stream";
import { S3Client, GetObjectCommand } from "@aws-sdk/client-s3";
import { handle, type Env, type ImagesBinding } from "../src/index";

const PORT = parseInt(process.env.PORT ?? "8787", 10);
const SIGNING_KEY = process.env.WORKER_SIGNING_KEY ?? "";
const BUCKET = process.env.R2_BUCKET ?? "art-dev";

const s3 = new S3Client({
  endpoint: process.env.S3_ENDPOINT ?? "http://minio:9000",
  region: "auto",
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.S3_ACCESS_KEY_ID ?? "minioadmin",
    secretAccessKey: process.env.S3_SECRET_ACCESS_KEY ?? "minioadmin",
  },
});

// Minimal R2 shim — handle() only calls .get(); other methods are typed but unused.
const r2 = {
  async get(key: string) {
    try {
      const out = await s3.send(new GetObjectCommand({ Bucket: BUCKET, Key: key }));
      const bytes = await out.Body!.transformToByteArray();
      return {
        body: Readable.toWeb(Readable.from(Buffer.from(bytes))) as ReadableStream,
      };
    } catch {
      return null;
    }
  },
} as unknown as Env["R2"];

// IMAGES stub — no real resize. Layer-B (Task 11) is what proves the real
// binding works; this stub just ensures privacy + Cache-Control logic flow
// through the production handler.
const images: ImagesBinding = {
  input(stream) {
    return {
      transform() { return this; },
      async output(opts: { format: string; quality?: number }) {
        return {
          response: () => new Response(stream, {
            status: 200,
            headers: { "Content-Type": opts.format },
          }),
        };
      },
    } as never;
  },
};

const env: Env = { R2: r2, IMAGES: images, WORKER_SIGNING_KEY: SIGNING_KEY };

const server = createServer(async (nodeReq, nodeRes) => {
  if (nodeReq.url === "/healthz") {
    nodeRes.writeHead(200, { "Content-Type": "text/plain" });
    nodeRes.end("ok");
    return;
  }

  const url = `http://localhost:${PORT}${nodeReq.url}`;
  const headers = new Headers();
  for (const [k, v] of Object.entries(nodeReq.headers)) {
    if (typeof v === "string") headers.set(k, v);
    else if (Array.isArray(v)) headers.set(k, v.join(","));
  }
  try {
    const resp = await handle(new Request(url, { method: nodeReq.method, headers }), env);
    const respHeaders: Record<string, string> = {};
    resp.headers.forEach((v, k) => { respHeaders[k] = v; });
    nodeRes.writeHead(resp.status, respHeaders);
    if (resp.body) {
      const reader = resp.body.getReader();
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        nodeRes.write(value);
      }
    }
    nodeRes.end();
  } catch (err) {
    nodeRes.writeHead(500, { "Content-Type": "text/plain" });
    nodeRes.end(`worker error: ${(err as Error).message}`);
  }
});

server.listen(PORT, () => console.log(`worker e2e server on :${PORT}`));
