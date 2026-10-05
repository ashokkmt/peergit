const assert = require('node:assert/strict');
const { createHash, randomBytes } = require('node:crypto');
const { createServer } = require('node:http');
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.CI ? {} : { channel: 'msedge' }) });
  const baseURL = process.env.PEERGIT_WEB_URL || 'http://127.0.0.1:3000';
  const identities = new Map();
  const provider = createServer(async (request, response) => {
    const url = new URL(request.url, 'http://127.0.0.1:8090');
    if (url.pathname === '/login/oauth/authorize') {
      const redirect = new URL(url.searchParams.get('redirect_uri'));
      const code = randomBytes(20).toString('hex');
      const id = Number(BigInt(`0x${randomBytes(6).toString('hex')}`));
      identities.set(code, { id, login: `test_${id}`, email: `github-${id}@users.noreply.test`, challenge: url.searchParams.get('code_challenge') });
      redirect.searchParams.set('code', code);
      redirect.searchParams.set('state', url.searchParams.get('state'));
      response.writeHead(302, { Location: redirect.toString() }).end();
      return;
    }
    if (url.pathname === '/login/oauth/access_token' && request.method === 'POST') {
      let body = '';
      for await (const chunk of request) body += chunk;
      const form = new URLSearchParams(body);
      const identity = identities.get(form.get('code'));
      const challenge = createHash('sha256').update(form.get('code_verifier') ?? '').digest('base64url');
      if (!identity || identity.challenge !== challenge) {
        response.writeHead(400, { 'Content-Type': 'application/json' }).end(JSON.stringify({ error: 'invalid_grant' }));
        return;
      }
      response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ access_token: form.get('code'), token_type: 'bearer' }));
      return;
    }
    const match = /^Bearer (.+)$/.exec(request.headers.authorization ?? '');
    const identity = match && identities.get(match[1]);
    if (identity && url.pathname === '/user') {
      response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ id: identity.id, login: identity.login, name: `Test Student ${identity.id}` }));
      return;
    }
    if (identity && url.pathname === '/user/emails') {
      response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify([{ email: identity.email, primary: true, verified: true }]));
      return;
    }
    response.writeHead(404).end();
  });
  await new Promise((resolve, reject) => { provider.once('error', reject); provider.listen(8090, '127.0.0.1', resolve); });
  try {
    for (const viewport of [{ width: 1280, height: 900 }, { width: 390, height: 844 }]) {
      const page = await browser.newPage({ viewport });
      await page.route('**/readyz', route => route.fulfill({ status: 200, headers: { 'X-Request-ID': 'phase1-browser-check' }, body: '{}' }));
      await page.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
      await page.route('**/api/v1/projects**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [], next_cursor: '' } }) }));
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
      await page.getByRole('heading', { name: 'No public projects found for “robotics”' }).waitFor();
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

    const signup = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await signup.goto(`${baseURL}/signup`);
    await signup.getByRole('heading', { name: 'Start with GitHub' }).waitFor();
    await signup.getByRole('link', { name: /Continue with GitHub/ }).waitFor();
    await signup.getByRole('navigation', { name: 'Account' }).getByRole('link', { name: 'Log in' }).waitFor();
    await signup.goto(`${baseURL}/login`);
    await signup.getByRole('heading', { name: 'Log in to PeerGit' }).waitFor();
    await signup.close();
    console.log('GitHub signup and login routes passed');

    const accountMenu = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await accountMenu.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'menu-csrf', user: { display_name: 'Test Student', account_type: 'campus', campus_status: 'verified', college_id: 'campus-1' } } }) }));
    await accountMenu.route('**/api/v1/projects**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await accountMenu.goto(baseURL);
    assert.equal(await accountMenu.getByRole('button', { name: /Create project/i }).count(), 0, 'homepage must not expose repository-free project creation');
    assert.equal(await accountMenu.locator('input').count(), 1, 'homepage should only expose the project search field');
    const menuSummary = accountMenu.locator('.account-menu summary');
    await menuSummary.click();
    await accountMenu.getByRole('link', { name: 'GitHub connections' }).waitFor();
    await accountMenu.keyboard.press('Escape');
    assert.equal(await accountMenu.locator('.account-menu').evaluate(element => element.open), false, 'Escape closes the account menu');
    assert.equal(await accountMenu.evaluate(() => document.activeElement.matches('.account-menu summary')), true, 'Escape returns focus to the menu button');
    await accountMenu.route('**/api/v1/me/github/repositories', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ id: 41003, full_name: 'test-owner/fixture', description: 'Synthetic repository', visibility: 'private', default_branch: 'main', topics: [], installation_id: '41001' }] } }) }));
    await accountMenu.route('**/api/v1/me/github/installations', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ id: 'local-install-1', installation_id: '41001', account: 'test-owner', target_type: 'User', status: 'active', confirmed_at: new Date().toISOString() }] } }) }));
    await accountMenu.route('**/api/v1/projects/import/github', async route => {
      assert.equal(route.request().postDataJSON().repository_id, 41003);
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authorization_url: 'http://127.0.0.1:8090/repository-authorize' } }) });
    });
    await accountMenu.goto(`${baseURL}/account/github`);
    await accountMenu.getByRole('heading', { name: 'Import a project from GitHub' }).waitFor();
    await accountMenu.getByText('test-owner/fixture').waitFor();
    await accountMenu.getByRole('button', { name: 'Import privately' }).click();
    await accountMenu.waitForURL('http://127.0.0.1:8090/repository-authorize');
    await accountMenu.close();
    console.log('Discovery-only homepage, accessible account menu, and private GitHub import initiation passed');

    const projectPage = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    projectPage.on('pageerror', error => console.error(`Project detail browser error: ${error.message}`));
    projectPage.on('console', message => { if (message.type() === 'error') console.error(`Project detail console error: ${message.text()}`); });
    await projectPage.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'project-csrf', user: { display_name: 'Test Owner', college_id: 'campus-1', terms_accepted: true, privacy_accepted: true } } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/repository', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { repository: { id: 'binding-1', full_name: 'test-owner/imported-project', html_url: 'https://github.com/test-owner/imported-project', description: 'Synthetic repo', default_branch: 'main', topics: [], languages: {}, readme_markdown: '', access_state: 'active', sync_health: 'current', limitations: 'LFS and submodule content is not captured.' }, contributions: [], snapshots: [] } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/applications', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await projectPage.route('**/api/v1/projects/imported-project', async route => {
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON();
        assert.equal(body.summary, 'Updated summary');
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'imported-project', version: body.lifecycle === 'active' ? 3 : 2 } }) });
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { project: { id: 'imported-project', title: 'Imported project', summary: 'Original summary', visibility: 'private', lifecycle: 'draft', project_type: 'open_source', recruiting: false, skills: [], version: 1 }, description: 'Original description', viewer_role: 'owner', members: [{ user_id: 'owner-1', display_name: 'Test Owner', role: 'owner' }] } }) });
      }
    });
    await projectPage.route('**/api/v1/projects/imported-project/roles', async route => {
      if (route.request().method() === 'POST') {
        assert.equal(route.request().postDataJSON().title, 'Rust contributor');
        await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: 'role-1', status: 'open' } }) });
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) });
      }
    });
    const projectResponse = await projectPage.goto(`${baseURL}/projects/imported-project`);
    try { await projectPage.getByRole('heading', { name: 'Imported project' }).waitFor({ timeout: 5000 }); }
    catch { throw new Error(`Imported project page did not render (HTTP ${projectResponse?.status()}): ${await projectPage.locator('body').innerText()}`); }
    await projectPage.getByRole('textbox', { name: 'Summary' }).fill('Updated summary');
    await projectPage.getByRole('button', { name: 'Save details' }).click();
    await projectPage.getByText('Project details saved.').waitFor();
    await projectPage.getByRole('button', { name: 'Publish to campus' }).click();
    await projectPage.getByText(/Project published to your campus/).waitFor();
    await projectPage.getByRole('textbox', { name: 'Role title' }).fill('Rust contributor');
    await projectPage.getByRole('textbox', { name: 'What will the member work on?' }).fill('Improve the project documentation.');
    await projectPage.getByRole('button', { name: 'Create opening' }).click();
    await projectPage.getByText('Team opening created.').waitFor();
    await projectPage.close();
    console.log('Imported project editing, recruiting, and explicit publication controls passed');

    const onboarding = await browser.newPage({ viewport: { width: 390, height: 844 } });
    let confirmationPosts = 0;
    let challengeRequested = false;
    await onboarding.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'browser-test-csrf' } }) }));
    await onboarding.route('**/api/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'browser-test-user', college_id: '', email: 'student@users.noreply.test', display_name: 'Browser Test Student', account_type: 'unverified', profile: {}, skills: [], consents: ['terms', 'privacy'], campus_admin: false, mfa_enabled: false } }) }));
    await onboarding.route('**/api/v1/me/onboarding', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { account_type: 'unverified', campus_id: '', campus_verified: false, ...(challengeRequested ? { pending_challenge: { challenge_id: 'browser-test-challenge', email: 'st***@local.edu', expires_at: new Date(Date.now() + 600_000).toISOString() } } : {}) } }) }));
    await onboarding.route('**/api/v1/me/campus-verification/challenges', async route => {
      assert.equal(route.request().postDataJSON().campus_email, 'student@local.edu');
      challengeRequested = true;
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { challenge_id: 'browser-test-challenge' } }) });
    });
    await onboarding.route('**/api/v1/me/campus-verification/confirm', async route => {
      confirmationPosts++;
      const body = route.request().postDataJSON();
      assert.equal(body.method, 'otp');
      assert.equal(body.challenge_id, 'browser-test-challenge');
      assert.equal(body.otp, '123456');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { verified: true } }) });
    });
    await onboarding.goto(`${baseURL}/onboarding`);
    await onboarding.getByRole('heading', { name: 'Verify your campus email' }).waitFor();
    await onboarding.getByRole('textbox', { name: 'Campus email', exact: true }).fill('student@local.edu');
    await onboarding.getByRole('button', { name: 'Send verification email' }).click();
    await onboarding.getByRole('status').getByText('A verification link and six-digit code were sent. Check your campus inbox.').waitFor();
    await onboarding.getByRole('link', { name: 'Enter its code' }).click();
    await onboarding.getByLabel('Six-digit code').fill('123456');
    assert.equal(confirmationPosts, 0, 'rendering the confirmation page must not consume the challenge');
    await onboarding.getByRole('button', { name: 'Confirm campus email' }).click();
    try {
      await onboarding.getByRole('status').getByText('Campus email verified.').waitFor({ timeout: 10000 });
    } catch (error) {
      const pageText = (await onboarding.locator('body').innerText()).replace(/\s+/g, ' ').slice(-1200);
      throw new Error(`OTP confirmation did not reach its success state (confirm POSTs: ${confirmationPosts}; URL: ${onboarding.url()}; page: ${pageText}). ${error.message}`);
    }
    assert.equal(confirmationPosts, 1);
    await onboarding.close();
    console.log('Campus onboarding and explicit OTP confirmation routes passed');

    const live = await browser.newPage({ viewport: { width: 390, height: 844 } });
    const liveCampusEmail = `student-${Date.now()}@local.edu`;
    await live.goto(`${baseURL}/signup`);
    await live.getByRole('link', { name: /Continue with GitHub/ }).click();
    await live.getByRole('heading', { name: 'Set up your PeerGit account' }).waitFor();
    await live.locator('input[type="checkbox"]').nth(0).check();
    await live.locator('input[type="checkbox"]').nth(1).check();
    await live.getByRole('button', { name: 'Save profile' }).click();
    await live.getByText('Your profile and consent choices are saved.').waitFor();
    await live.getByRole('textbox', { name: 'Campus email', exact: true }).fill(liveCampusEmail);
    await live.getByRole('button', { name: 'Send verification email' }).click();
    await live.getByRole('status').getByText('A message was sent to st***@local.edu').waitFor();
    let emailText = '';
    const deadline = Date.now() + 20_000;
    while (Date.now() < deadline) {
      const mail = await fetch(`http://127.0.0.1:8025/view/latest.txt?query=${encodeURIComponent(`to:${liveCampusEmail}`)}`);
      if (mail.ok) {
        emailText = await mail.text();
        if (emailText.includes('six-digit code')) break;
      }
      await new Promise(resolve => setTimeout(resolve, 500));
    }
    const verificationLink = /https?:\/\/[^\s<>]+\/onboarding\/verify#token=[A-Za-z0-9_-]+/.exec(emailText)?.[0];
    assert(verificationLink, 'Mailpit did not capture the verification message from the API worker');
    let confirmPosts = 0;
    live.on('request', request => { if (request.method() === 'POST' && request.url().includes('/api/v1/me/campus-verification/confirm')) confirmPosts++; });
    await live.goto(verificationLink);
    await live.getByRole('heading', { name: 'Check your campus email' }).waitFor();
    assert(!live.url().includes('token='), 'verification token should be removed from the browser URL');
    await new Promise(resolve => setTimeout(resolve, 250));
    assert.equal(confirmPosts, 0, 'opening an email link must not consume the challenge');
    await live.getByRole('button', { name: 'Confirm campus email' }).click();
    await live.getByText('Campus email verified.').waitFor();
    await live.goto(`${baseURL}/onboarding`);
    await live.getByRole('heading', { name: 'Campus verified' }).waitFor();
    await live.close();
    console.log('Fake GitHub OAuth, worker, Mailpit, link confirmation, and session refresh passed');
  } finally {
    await browser.close();
    await new Promise(resolve => provider.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
