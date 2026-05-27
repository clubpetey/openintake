// Tiny webhook receiver for the Phase 1 smoke.
// Logs every POST body to stdout as formatted JSON.
// Usage: node server.mjs   (listens on :9099)

import { createServer } from 'node:http';

const PORT = 9099;
const PATH = '/intake';

const server = createServer((req, res) => {
  if (req.method === 'POST' && req.url === PATH) {
    let body = '';
    req.on('data', (chunk) => { body += chunk; });
    req.on('end', () => {
      const timestamp = new Date().toISOString();
      console.log(`\n[${timestamp}] POST ${PATH}`);
      try {
        const parsed = JSON.parse(body);
        console.log(JSON.stringify(parsed, null, 2));
      } catch {
        console.log('(non-JSON body):', body);
      }
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ received: true }));
    });
  } else {
    res.writeHead(404);
    res.end('Not found');
  }
});

server.listen(PORT, () => {
  console.log(`Webhook receiver listening on http://localhost:${PORT}${PATH}`);
});
