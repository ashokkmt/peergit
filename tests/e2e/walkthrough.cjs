const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { chromium } = require('playwright');

(async () => {
  const root = path.resolve(__dirname, '../..');
  const output = path.join(root, '.proofs');
  fs.mkdirSync(output, { recursive: true });
  // Playwright launches a fresh isolated profile, never the user's existing browser.
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    for (const viewport of [{width:1280,height:900}, {width:390,height:844}]) {
      const page = await browser.newPage({viewport});
      await page.goto(pathToFileURL(path.join(root, 'docs/verification/walkthrough.html')).href);
      assert.equal(await page.evaluate(() => window.innerWidth), viewport.width);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'horizontal overflow');
      assert(await page.evaluate(() => [...document.querySelectorAll('input,select,textarea')].every(element => document.querySelector(`label[for="${element.id}"]`))), 'missing form label');
      await page.keyboard.press('Tab');
      assert.equal(await page.evaluate(() => document.activeElement.textContent), 'Skip to walkthrough');
      await page.keyboard.press('Enter');
      assert.equal(await page.evaluate(() => location.hash), '#content');
      assert.equal(await page.locator('#email').evaluate(element => element.checkValidity()), false);
      await page.getByLabel('Campus email').fill('fixture@synthetic.example');
      await page.getByRole('button', {name:'Preview onboarding'}).click();
      assert.match(await page.locator('#join-status').textContent(), /No account was created/);
      await page.getByLabel('Why would you like to join?').fill('I can document the project.');
      await page.getByRole('button', {name:'Preview private application'}).click();
      assert.match(await page.locator('#application-status').textContent(), /Nothing was sent/);
      await page.getByLabel('Preview installation state').selectOption('revoked');
      assert.match(await page.locator('#provider-status').textContent(), /Future synchronization and capture stop/);
      await page.getByLabel('Preview capture state').selectOption('failed');
      await page.getByRole('button', {name:'Preview retry of the same SHA'}).click();
      assert.match(await page.locator('#retry-status').textContent(), /original SHA/);
      await page.getByLabel('Preview capture state').selectOption('verified');
      assert.equal(await page.locator('#retry').isVisible(), false);
      assert.match(await page.locator('#capture-status').textContent(), /stored bytes were reread/);
      await page.evaluate(() => window.scrollTo(0,0));
      await page.screenshot({path:path.join(output, `walkthrough-${viewport.width}.png`),fullPage:true});
      await page.close();
      console.log(`Walkthrough passed at ${viewport.width} × ${viewport.height}`);
    }
  } finally { await browser.close(); }
})().catch(error => {console.error(error);process.exitCode=1;});
