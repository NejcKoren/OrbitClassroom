// Decoder for the Orbit delta-tile screen stream (see internal/proto/stream.go).
// Frames arrive through fetch() streaming; every rectangle is a JPEG that is
// decoded with createImageBitmap (hardware accelerated) and drawn onto a canvas.

class ByteQueue {
  constructor() { this.buf = new Uint8Array(1 << 20); this.start = 0; this.end = 0; }
  get length() { return this.end - this.start; }
  push(chunk) {
    if (this.end + chunk.length > this.buf.length) {
      const len = this.length;
      if (len + chunk.length > this.buf.length) {
        const nb = new Uint8Array(Math.max(this.buf.length * 2, len + chunk.length));
        nb.set(this.buf.subarray(this.start, this.end));
        this.buf = nb;
      } else {
        this.buf.copyWithin(0, this.start, this.end);
      }
      this.start = 0; this.end = len;
    }
    this.buf.set(chunk, this.end);
    this.end += chunk.length;
  }
  u8(off) { return this.buf[this.start + off]; }
  u16(off) { const p = this.start + off; return (this.buf[p] << 8) | this.buf[p + 1]; }
  u32(off) { const p = this.start + off; return ((this.buf[p] << 24) >>> 0) + ((this.buf[p + 1] << 16) | (this.buf[p + 2] << 8) | this.buf[p + 3]); }
  take(n) {
    const out = this.buf.slice(this.start, this.start + n);
    this.start += n;
    if (this.start === this.end) { this.start = 0; this.end = 0; }
    return out;
  }
}

class OrbitStream {
  constructor(canvas, onFrame) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d', { alpha: false, desynchronized: true });
    this.onFrame = onFrame || (() => {});
    this.bytes = 0; this.frames = 0; this.stopped = false;
    this.width = 0; this.height = 0;
  }

  // frameLength returns the byte length of the next complete frame, or 0.
  frameLength(q) {
    if (q.length < 10) return 0;
    if (q.u8(0) !== 0x4f || q.u8(1) !== 0x46) throw new Error('bad stream');
    const n = q.u16(8);
    let need = 10;
    for (let i = 0; i < n; i++) {
      if (q.length < need + 12) return 0;
      need += 12 + q.u32(need + 8);
      if (q.length < need) return 0;
    }
    return need;
  }

  async start(url) {
    this.abort = new AbortController();
    const res = await fetch(url, { signal: this.abort.signal, cache: 'no-store' });
    if (!res.ok) {
      let msg = 'HTTP ' + res.status;
      try { msg = (await res.json()).error || msg; } catch {}
      throw new Error(msg);
    }
    const reader = res.body.getReader();
    const q = new ByteQueue();
    while (!this.stopped) {
      const { value, done } = await reader.read();
      if (done) break;
      this.bytes += value.length;
      q.push(value);
      for (;;) {
        const len = this.frameLength(q);
        if (!len) break;
        await this.draw(q.take(len));
      }
    }
  }

  async draw(buf) {
    const dv = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    const w = dv.getUint16(4), h = dv.getUint16(6), n = dv.getUint16(8);
    if (n === 0) return;
    if (w !== this.width || h !== this.height) {
      this.canvas.width = w; this.canvas.height = h;
      this.width = w; this.height = h;
    }
    const jobs = [];
    let off = 10;
    for (let i = 0; i < n; i++) {
      const x = dv.getUint16(off), y = dv.getUint16(off + 2), len = dv.getUint32(off + 8);
      const blob = new Blob([buf.subarray(off + 12, off + 12 + len)], { type: 'image/jpeg' });
      jobs.push(createImageBitmap(blob).then(b => ({ b, x, y })).catch(() => null));
      off += 12 + len;
    }
    const parts = await Promise.all(jobs);
    for (const p of parts) {
      if (!p) continue;
      this.ctx.drawImage(p.b, p.x, p.y);
      p.b.close && p.b.close();
    }
    this.frames++;
    this.onFrame(w, h);
  }

  stop() {
    this.stopped = true;
    if (this.abort) this.abort.abort();
  }
}
