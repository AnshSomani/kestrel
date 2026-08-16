const http = require('http');

const port = process.env.PORT || '9999';
let failRate = parseFloat(process.env.FAIL_RATE || '0.0');
let slowRate = parseFloat(process.env.SLOW_RATE || '0.0');
const secret = process.env.SECRET || '';

console.log(`\x1b[36m╔══════════════════════════════════════╗\x1b[0m`);
console.log(`\x1b[36m║   🎯 Kestrel Webhook Target Server   ║\x1b[0m`);
console.log(`\x1b[36m╚══════════════════════════════════════╝\x1b[0m`);
console.log(`  Port:      ${port}`);
console.log(`  Fail Rate: ${(failRate * 100).toFixed(0)}%`);
console.log(`  Slow Rate: ${(slowRate * 100).toFixed(0)}%`);
console.log(`  HMAC:      ${secret ? 'enabled' : 'disabled'}\n`);

let counter = 0;

const server = http.createServer((req, res) => {
  const urlObj = new URL(req.url, `http://${req.headers.host || 'localhost'}`);

  if (req.method === 'POST' && urlObj.pathname === '/config') {
    const f = urlObj.searchParams.get('failRate');
    if (f !== null) failRate = parseFloat(f);
    const s = urlObj.searchParams.get('slowRate');
    if (s !== null) slowRate = parseFloat(s);

    console.log(`\x1b[36m⚙️ Config updated: FailRate=${(failRate * 100).toFixed(0)}% SlowRate=${(slowRate * 100).toFixed(0)}%\x1b[0m`);
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ status: 'config updated', failRate, slowRate }));
  }

  counter++;
  const reqNum = counter;
  const start = Date.now();

  const chunks = [];
  req.on('data', (chunk) => chunks.push(chunk));
  req.on('end', async () => {
    const bodyStr = Buffer.concat(chunks).toString('utf8');

    // Simulate slow response
    const isSlow = Math.random() < slowRate;
    if (isSlow) {
      const delay = Math.floor(2000 + Math.random() * 3000);
      console.log(`\x1b[33m[#${reqNum}] ⏳ Simulating slow response (${delay}ms)...\x1b[0m`);
      await new Promise((resolve) => setTimeout(resolve, delay));
    }

    // Simulate failure
    const isFail = Math.random() < failRate;
    const statusCode = isFail ? 500 : 200;
    const duration = Date.now() - start;

    let statusColor = '\x1b[32m'; // green
    let statusIcon = '✓';
    if (statusCode >= 500) {
      statusColor = '\x1b[31m'; // red
      statusIcon = '✗';
    } else if (isSlow) {
      statusColor = '\x1b[33m'; // yellow
      statusIcon = '⚡';
    }

    let preview = bodyStr.replace(/\n/g, ' ');
    if (preview.length > 120) preview = preview.slice(0, 120) + '...';

    const kestrelHeaders = Object.entries(req.headers)
      .filter(([k]) => k.startsWith('x-kestrel'))
      .map(([k, v]) => `${k}=${v}`)
      .join(', ');

    console.log(`${statusColor}[#${reqNum}] ${statusIcon} ${req.method} ${urlObj.pathname} → ${statusCode} (${duration}ms)\x1b[0m`);
    if (kestrelHeaders) console.log(`  \x1b[90mHeaders: ${kestrelHeaders}\x1b[0m`);
    console.log(`  \x1b[90mBody: ${preview}\x1b[0m`);

    res.writeHead(statusCode, { 'Content-Type': 'application/json' });
    if (statusCode === 200) {
      res.end(JSON.stringify({ status: 'received', request_number: reqNum }));
    } else {
      res.end(JSON.stringify({ error: 'simulated failure', request_number: reqNum }));
    }
  });
});

server.listen(parseInt(port), () => {
  console.log(`Webhook target listening on :${port}`);
});
