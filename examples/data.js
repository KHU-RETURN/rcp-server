let pending = '';
function readLine() {
  for (;;) {
    const newline = pending.indexOf('\n');
    if (newline >= 0) {
      const line = pending.slice(0, newline);
      pending = pending.slice(newline + 1);
      return JSON.parse(line);
    }
    const chunk = new Uint8Array(4096);
    const count = Javy.IO.readSync(0, chunk);
    if (count === 0) throw new Error('stdin closed');
    pending += new TextDecoder().decode(chunk.subarray(0, count));
  }
}

function send(value) {
  Javy.IO.writeSync(1, new TextEncoder().encode(`${JSON.stringify(value)}\n`));
}

function db(operation) {
  send({ $rcp: 'db', ...operation });
  const reply = readLine();
  if (!reply.ok) throw new Error(reply.error);
  return reply;
}

const event = readLine();
db({ op: 'put', collection: 'visits', key: 'count', value: 1 });
const item = db({ op: 'get', collection: 'visits', key: 'count' }).item;
send({ statusCode: 200, headers: { 'content-type': 'application/json' }, body: JSON.stringify({ count: item.value }) });
