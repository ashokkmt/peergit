const assert = require('node:assert/strict');
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.CI ? {} : { channel: 'msedge' }) });
  const baseURL = process.env.PEERGIT_WEB_URL || 'http://127.0.0.1:3000';
  try {
    for (const viewport of [{ width: 1280, height: 900 }, { width: 390, height: 844 }]) {
      const page = await browser.newPage({ viewport });
      await page.route('**/readyz', route => route.fulfill({ status: 200, headers: { 'X-Request-ID': 'phase1-browser-check' }, body: '{}' }));
      await page.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
      await page.goto(baseURL);
      assert.equal(await page.evaluate(() => window.innerWidth), viewport.width);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'horizontal overflow');
      assert(await page.evaluate(() => [...document.querySelectorAll('input,select,textarea')].every(element => element.labels.length > 0)), 'missing form label');
      await page.keyboard.press('Tab');
      assert.equal((await page.evaluate(() => document.activeElement.textContent)).trim(), 'Skip to content');
      await page.keyboard.press('Enter');
      assert.equal(await page.evaluate(() => location.hash), '#main-content');
      await page.getByRole('heading', { name: 'Good projects grow with good people.' }).waitFor();
      const readyStatus = page.locator('.service-status');
      await readyStatus.getByText('Local API is ready').waitFor();
      assert.match(await readyStatus.textContent(), /Request ID: phase1-browser-check/);
      const search = page.getByRole('searchbox', { name: 'Search projects' });
      await search.fill('robotics');
      await page.getByRole('button', { name: 'Search' }).click();
      await page.getByRole('heading', { name: 'Sign in to explore campus projects' }).waitFor();
      await page.close();
      console.log(`Walkthrough passed at ${viewport.width} × ${viewport.height}`);
    }
    const unavailable = await browser.newPage();
    await unavailable.route('**/readyz', route => route.fulfill({ status: 503, headers: { 'X-Request-ID': 'phase1-unavailable-check' }, body: '{}' }));
    await unavailable.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
    await unavailable.goto(baseURL);
    const unavailableStatus = unavailable.locator('.service-status');
    await unavailableStatus.getByText('The API is unavailable. You can still explore this preview.').waitFor();
    assert.match(await unavailableStatus.textContent(), /Request ID: phase1-unavailable-check/);
    await unavailable.close();
    console.log('Service-unavailable state passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
