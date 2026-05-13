const { chromium } = require('playwright');
const path = require('path');

const CHROME_PATH = '/home/rain/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome';

(async () => {
  const browser = await chromium.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  });

  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
  });
  const page = await context.newPage();

  // Track WebSocket messages
  const wsMessages = [];
  page.on('console', msg => {
    const text = msg.text();
    if (text.includes('[WS]') || text.includes('progress') || text.includes('sync')) {
      wsMessages.push({ time: Date.now(), text });
    }
  });

  // Navigate to the app
  console.log('[TEST] Opening page...');
  await page.goto('http://localhost:10086', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // Screenshot 1: Initial state
  await page.screenshot({ path: '/tmp/progress-01-initial.png', fullPage: true });
  console.log('[TEST] Screenshot 1: initial state');

  // Create a test file to sync
  const { execSync } = require('child_process');
  try {
    execSync('dd if=/dev/urandom of=/home/rain/Downloads/progress-test.bin bs=1M count=80 2>/dev/null');
    console.log('[TEST] Created 80MB test file');
  } catch (e) {
    console.log('[TEST] Failed to create test file:', e.message);
  }

  // Wait a moment for file system to settle
  await page.waitForTimeout(1000);

  // Trigger sync via API from the browser
  console.log('[TEST] Triggering sync...');
  const syncResult = await page.evaluate(async () => {
    const res = await fetch('/api/v1/sync', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pair_id: 5 }),
    });
    return { status: res.status, body: await res.text() };
  });
  console.log('[TEST] Sync trigger result:', syncResult);

  // Rapid screenshots during sync
  for (let i = 2; i <= 15; i++) {
    await page.waitForTimeout(1000);
    const num = String(i).padStart(2, '0');
    await page.screenshot({ path: `/tmp/progress-${num}.png`, fullPage: true });
    console.log(`[TEST] Screenshot ${num} taken`);

    // Check if any active sync indicators exist on the page
    const progressInfo = await page.evaluate(() => {
      // Check for any elements with progress-related text
      const body = document.body.innerText;
      const hasSyncing = body.includes('Syncing') || body.includes('syncing');
      const hasProgress = body.includes('progress') || body.includes('Progress');
      const hasActive = body.includes('Active') || body.includes('active');

      // Check for progress bar elements
      const progressBars = document.querySelectorAll('[role="progressbar"]');
      const barCount = progressBars.length;

      // Check for any animated/moving elements
      const animating = document.querySelectorAll('[class*="animate"], [class*="progress"], [class*="sync"]');
      const animCount = animating.length;

      return { hasSyncing, hasProgress, hasActive, barCount, animCount };
    });
    console.log(`[TEST] Page state at ${num}:`, JSON.stringify(progressInfo));
  }

  console.log('[TEST] WS messages captured:', wsMessages.length);
  wsMessages.forEach(m => console.log(`  [WS] ${m.text}`));

  await browser.close();
  console.log('[TEST] Done');
})().catch(e => {
  console.error('[TEST] Error:', e);
  process.exit(1);
});
