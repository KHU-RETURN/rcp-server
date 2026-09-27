const chunks = [];
for (;;) {
  const buffer = new Uint8Array(4096);
  const count = Javy.IO.readSync(0, buffer);
  if (count === 0) break;
  chunks.push(new TextDecoder().decode(buffer.subarray(0, count)));
}
Javy.IO.writeSync(1, new TextEncoder().encode(chunks.join('')));
