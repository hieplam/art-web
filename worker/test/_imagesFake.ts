// worker/test/_imagesFake.ts
// Records each transform pipeline call so tests can assert width/fmt/q
// shape. The output is a pass-through of the source stream — Layer-A tests
// don't verify pixel-level resize (that's Layer-B, Task 11).
export type ImagesCall = { width?: number; format: string; quality: number };

export class ImagesFake {
  public calls: ImagesCall[] = [];

  input(stream: ReadableStream): ImagesPipeline {
    return new ImagesPipeline(this, stream);
  }
}

class ImagesPipeline {
  private widthHint?: number;
  constructor(private fake: ImagesFake, private stream: ReadableStream) {}
  transform(opts: { width?: number }): ImagesPipeline {
    this.widthHint = opts.width;
    return this;
  }
  async output(opts: { format: string; quality: number }): Promise<{ response(): Response }> {
    this.fake.calls.push({ width: this.widthHint, format: opts.format, quality: opts.quality });
    return {
      response: () => new Response(this.stream, {
        status: 200,
        headers: { "Content-Type": opts.format },
      }),
    };
  }
}
