import { defineConfig } from '@playwright/test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:net';

const web = dirname(fileURLToPath(import.meta.url));
const data = process.env.OANH_E2E_DATA ?? mkdtempSync(join(tmpdir(), 'oanh-live-e2e-'));
process.env.OANH_E2E_DATA = data;
const quote = (value: string) => `'${value.replace(/'/g, "'\\''")}'`;
const port = process.env.OANH_E2E_PORT ?? await new Promise<string>((resolve, reject) => {
  const listener = createServer();
  listener.once('error', reject);
  listener.listen(0, '127.0.0.1', () => {
    const address = listener.address();
    if (!address || typeof address === 'string') { listener.close(); reject(new Error('Cannot allocate test port')); return; }
    listener.close(() => resolve(String(address.port)));
  });
});
process.env.OANH_E2E_PORT = port;
const origin = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './e2e/live',
  workers: 1,
  fullyParallel: false,
  reporter: 'list',
  timeout: 45_000,
  use: { baseURL: origin, viewport: { width: 1440, height: 1000 }, trace: 'retain-on-failure', launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH } },
  webServer: { command: `go run ./cmd/server -addr 127.0.0.1:${port} -data ${quote(data)} -web ${quote(join(web, 'dist'))}`, cwd: resolve(web, '../server'), url: `${origin}/api/v1/session`, reuseExistingServer: false, timeout: 60_000 },
});
