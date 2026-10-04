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

    const repositoryUI = await browser.newPage({ viewport: { width: 390, height: 844 } });
    const projectID = '00000000-0000-7000-8000-000000000123';
    const repositoryID = '00000000-0000-7000-8000-000000000124';
    const bindingID = '00000000-0000-7000-8000-000000000125';
    const snapshotID = '00000000-0000-7000-8000-000000000126';
    let repositoryBound = false;
    let captureRecorded = false;
    let repositoryReadsAfterCapture = 0;
    await repositoryUI.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'repository-test-csrf', user: { college_id: '00000000-0000-7000-8000-000000000001' } } }) }));
    await repositoryUI.route('**/api/v1/projects', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ id: projectID, slug: 'fixture', title: 'Repository fixture', summary: 'Test repository flow', project_type: 'side_project', visibility: 'campus', lifecycle: 'active', version: 1, recruiting: false, skills: [] }] } }) }));
    await repositoryUI.route('**/api/v1/my/project-invitations', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await repositoryUI.route('**/api/v1/my/applications', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await repositoryUI.route(`**/api/v1/projects/${projectID}`, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { project: { id: projectID, slug: 'fixture', title: 'Repository fixture', summary: 'Test repository flow', project_type: 'side_project', visibility: 'campus', lifecycle: 'active', version: 1, recruiting: false, skills: [] }, description: '', viewer_role: 'owner', members: [] } }) }));
    await repositoryUI.route(`**/api/v1/projects/${projectID}/roles`, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await repositoryUI.route(`**/api/v1/projects/${projectID}/applications`, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await repositoryUI.route(`**/api/v1/projects/${projectID}/github/repositories`, async route => {
      if (route.request().method() === 'GET') {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ installation_id: '41001', id: 41003, full_name: 'campus/fixture', private: true, visibility: 'private', default_branch: 'main' }] } }) });
      } else {
        assert.equal(route.request().postDataJSON().repository_id, 41003);
        repositoryBound = true;
        await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { repository_id: repositoryID, binding_id: bindingID, state: 'sync_pending' } }) });
      }
    });
    await repositoryUI.route(`**/api/v1/projects/${projectID}/repository`, route => {
      if (!repositoryBound) {
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { repository: null, contributions: [], snapshots: [] } }) });
      }
      let snapshots = [];
      if (captureRecorded) {
        repositoryReadsAfterCapture++;
        snapshots = [{ id: snapshotID, ref: 'main', commit_sha: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', state: repositoryReadsAfterCapture > 1 ? 'verified' : 'pending', receipt_at: new Date().toISOString(), archive_bytes: repositoryReadsAfterCapture > 1 ? 1024 : null, retry_available: false }];
      }
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { repository: { id: bindingID, full_name: 'campus/fixture', access_state: 'active', sync_health: 'healthy', capabilities: { contents: 'read', pull_requests: 'read', issues: 'read' }, limitations: 'Git LFS objects and submodule contents are not independently captured.' }, contributions: [], snapshots } }) });
    });
    await repositoryUI.route(`**/api/v1/projects/${projectID}/repository/snapshots`, async route => {
      assert.equal(route.request().postDataJSON().binding_id, bindingID);
      assert.equal(route.request().postDataJSON().ref, 'main');
      assert(route.request().headers()['idempotency-key']);
      captureRecorded = true;
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: snapshotID, commit_sha: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', state: 'pending' } }) });
    });
    await repositoryUI.goto(baseURL);
    await repositoryUI.getByRole('button', { name: 'View team and roles' }).click();
    await repositoryUI.getByRole('heading', { name: 'Repository' }).waitFor();
    assert(await repositoryUI.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'repository panel has horizontal overflow on mobile');
    await repositoryUI.getByRole('button', { name: 'Choose from a connected installation' }).click();
    await repositoryUI.getByLabel('Select a repository accessible to your campus installation').selectOption('41001:41003');
    await repositoryUI.getByRole('button', { name: 'Connect repository' }).click();
    await repositoryUI.getByText('campus/fixture', { exact: true }).waitFor();
    await repositoryUI.getByRole('button', { name: 'Record snapshot' }).click();
    await repositoryUI.getByText('pending', { exact: true }).waitFor();
    await repositoryUI.getByRole('link', { name: 'Download verified archive' }).waitFor({ timeout: 12000 });
    assert.equal(await repositoryUI.getByRole('link', { name: 'Download verified archive' }).getAttribute('href'), `/api/v1/projects/${projectID}/repository/snapshots/${snapshotID}/download`);
    await repositoryUI.close();
    console.log('Repository selection, exact-SHA receipt progress, mobile layout, and verified-download state passed');

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
