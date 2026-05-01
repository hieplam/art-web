// worker/src/index.ts
export default {
  async fetch(_req: Request, _env: unknown): Promise<Response> {
    return new Response("not yet implemented", { status: 501 });
  },
};
