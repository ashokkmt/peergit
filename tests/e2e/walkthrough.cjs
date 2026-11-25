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
    for (const viewport of [{ width: 1280, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 700 }]) {
      const page = await browser.newPage({ viewport });
      let publicItems = [];
      await page.route('**/readyz', route => route.fulfill({ status: 200, headers: { 'X-Request-ID': 'phase1-browser-check' }, body: '{}' }));
      await page.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
      await page.route('**/api/v1/projects**', route => {
        const term = new URL(route.request().url()).searchParams.get('q') || '';
        const items = term ? publicItems.filter(item => item.title.toLowerCase().includes(term.toLowerCase())) : publicItems;
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items, next_cursor: '' } }) });
      });
      await page.emulateMedia({ colorScheme: 'light' });
      await page.goto(baseURL);
      assert.equal(await page.evaluate(() => window.innerWidth), viewport.width);
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'horizontal overflow');
      assert(await page.evaluate(() => [...document.querySelectorAll('input,select,textarea')].every(element => element.labels.length > 0)), 'missing form label');
      await page.keyboard.press('Tab');
      assert.equal((await page.evaluate(() => document.activeElement.textContent)).trim(), 'Skip to content');
      await page.keyboard.press('Enter');
      assert.equal(await page.evaluate(() => location.hash), '#main-content');
      await page.getByRole('heading', { name: /Build something\s*worth sharing/ }).waitFor();
      assert.equal(await page.evaluate(() => document.documentElement.dataset.theme || 'system'), 'system');
      assert.equal(await page.getByRole('combobox', { name: 'Appearance' }).count(), 0, 'direct pages should use theme toggle button instead of dropdown');
      const toggle = page.getByRole('button', { name: 'Toggle color theme' });
      await toggle.click();
      await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
      await page.waitForFunction(() => document.querySelector('link[rel=manifest]').getAttribute('href') === '/manifest-dark.json');
      assert.equal(await page.locator('link[rel=icon]').getAttribute('href'), '/icons/favicon-dark.ico');
      await page.reload();
      await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
      await toggle.click();
      await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
      await page.waitForFunction(() => document.querySelector('link[rel=manifest]').getAttribute('href') === '/manifest-light.json');
      assert.equal(await page.locator('link[rel=icon]').getAttribute('href'), '/icons/favicon-light.ico');
      const selectedManifest = await page.request.get(baseURL + '/manifest-light.json');
      assert.equal(selectedManifest.ok(), true);
      assert.equal((await selectedManifest.json()).icons[0].src, '/icons/icon-192-light.png');
      const readyStatus = page.locator('.service-status');
      await readyStatus.getByText('Local API is ready').waitFor();
      assert.match(await readyStatus.textContent(), /Request ID: phase1-browser-check/);
      await page.getByRole('heading', { name: 'Be the first to share a public project' }).waitFor();
      assert.equal(await page.getByRole('searchbox', { name: 'Search projects' }).count(), 0, 'homepage hides search when zero public projects exist');
      publicItems = [{ id: 'sample-pub', title: 'Campus Transit Board', summary: 'Live arrivals', project_type: 'civic_campus', visibility: 'public', recruiting: true, skills: ['Go'] }];
      await page.reload();
      const search = page.getByRole('searchbox', { name: 'Search projects' });
      await search.fill('robotics');
      await page.getByRole('button', { name: 'Search' }).click();
      await page.getByRole('heading', { name: 'No public projects found for “robotics”' }).waitFor();
      await page.close();
      console.log(`Walkthrough passed at ${viewport.width} × ${viewport.height}`);
    }
    const settingsTheme = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    await settingsTheme.emulateMedia({ colorScheme: 'dark' });
    await settingsTheme.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
    await settingsTheme.goto(`${baseURL}/account/settings`);
    const appearance = settingsTheme.getByRole('combobox', { name: 'Appearance' });
    for (const theme of ['dark', 'light', 'system']) {
      await appearance.selectOption(theme);
      const expected = theme === 'system' ? 'dark' : theme;
      await settingsTheme.waitForFunction(expected => document.querySelector('link[rel=manifest]').getAttribute('href') === '/manifest-' + expected + '.json', expected);
      await settingsTheme.reload();
      await settingsTheme.waitForFunction(theme => (document.documentElement.dataset.theme || 'system') === theme, theme);
    }
    await settingsTheme.close();

    const unavailable = await browser.newPage();
    await unavailable.route('**/readyz', route => route.fulfill({ status: 503, headers: { 'X-Request-ID': 'phase1-unavailable-check' }, body: '{}' }));
    await unavailable.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: false } }) }));
    await unavailable.route('**/api/v1/projects**', route => route.fulfill({ status: 502, contentType: 'text/html', body: '<!DOCTYPE html><html><body>Bad Gateway</body></html>' }));
    await unavailable.goto(baseURL);
    const unavailableStatus = unavailable.locator('.service-status');
    await unavailableStatus.getByText('The API is unavailable. You can still explore this preview.').waitFor();
    assert.match(await unavailableStatus.textContent(), /Request ID: phase1-unavailable-check/);
    await unavailable.getByRole('alert').getByText("Couldn't fetch public projects right now").waitFor();
    await unavailable.close();
    console.log('Service-unavailable state passed');

    const previews = await browser.newPage({ viewport: { width: 390, height: 844 } });
    for (const [path, heading] of [
      ['/preview/native', 'PeerGit hosted repositories'],
      ['/inbox', 'Inbox'],
      ['/projects/sample/work', 'Tasks & milestones'],
      ['/u/example-student', 'Example Student'],
      ['/campus/overview', 'Campus overview'],
      ['/campus/reports', 'Cohorts, verification & reports'],
    ]) {
      await previews.goto(baseURL + path);
      await previews.getByRole('heading', { level: 1, name: heading }).waitFor();
      await previews.getByText('Will get implemented in a later phase', { exact: false }).first().waitFor();
      assert(await previews.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `horizontal overflow on ${path}`);
    }
    await previews.close();

    const signup = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await signup.goto(`${baseURL}/signup`);
    await signup.getByRole('heading', { name: 'Start with GitHub' }).waitFor();
    await signup.getByRole('link', { name: /Continue with GitHub/ }).waitFor();
    await signup.getByRole('navigation', { name: 'Account' }).getByRole('link', { name: 'Sign in' }).waitFor();
    await signup.goto(`${baseURL}/login`);
    await signup.getByRole('heading', { name: 'Log in to PeerGit' }).waitFor();
    await signup.close();
    console.log('GitHub signup and login routes passed');

    const accountMenu = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await accountMenu.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'menu-csrf', user: { display_name: 'Test Student', account_type: 'campus', campus_status: 'verified', college_id: 'campus-1' } } }) }));
    await accountMenu.route('**/api/v1/projects**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    await accountMenu.goto(baseURL);
    assert.equal(await accountMenu.getByRole('button', { name: /Create project/i }).count(), 0, 'homepage must not expose repository-free project creation');
    assert.equal(await accountMenu.locator('input').count(), 0, 'homepage should hide project search when zero public projects exist');
    await accountMenu.getByRole('link', { name: 'Add your project' }).waitFor();
    const menuSummary = accountMenu.locator('.account-menu summary');
    await menuSummary.click();
    for (const label of ['Profile', 'My projects', 'GitHub connection', 'Account settings']) {
      await accountMenu.locator('.account-menu').getByRole('link', { name: label, exact: true }).waitFor();
    }
    await accountMenu.keyboard.press('Escape');
    assert.equal(await accountMenu.locator('.account-menu').evaluate(element => element.open), false, 'Escape closes the account menu');
    assert.equal(await accountMenu.evaluate(() => document.activeElement.matches('.account-menu summary')), true, 'Escape returns focus to the menu button');
    await accountMenu.route('**/api/v1/me/github/repositories', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ id: 41003, installation_id: 'local-install-1', full_name: 'test-owner/fixture', description: 'Synthetic repository', visibility: 'private', default_branch: 'main', topics: [], expires_at: new Date(Date.now()+60000).toISOString() }], expired_count: 0 } }) }));
    await accountMenu.route('**/api/v1/me/github/installations', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ id: 'local-install-1', installation_id: '41001', account: 'test-owner', target_type: 'User', status: 'active', confirmed_at: new Date().toISOString() }] } }) }));
    let retainedProjects = [];
    await accountMenu.route('**/api/v1/me/github/retained-projects', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: retainedProjects } }) }));
    await accountMenu.route('**/api/v1/me/github/installations/local-install-1/refresh', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authorization_url: 'http://127.0.0.1:8090/repository-authorize' } }) }));
    await accountMenu.route('**/api/v1/me/github/installations/local-install-1/imports', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{title:'Imported fixture'}] } }) }));
    await accountMenu.route('**/api/v1/me/github/installations/local-install-1/revoke', route => {
      const deleteAfter = new Date(Date.now() + 30 * 86400000).toISOString();
      retainedProjects = [{ id: 'imported-fixture', title: 'Imported fixture', delete_after: deleteAfter }];
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { retained_projects: ['imported-fixture'], delete_after: deleteAfter } }) });
    });
    await accountMenu.route('**/api/v1/projects/imported-fixture/github/restore', route => {
      assert.equal(route.request().method(), 'POST');
      retainedProjects = [];
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'imported-fixture', visibility: 'private' } }) });
    });
    await accountMenu.route('**/api/v1/projects/import/github', async route => {
      assert.equal(route.request().postDataJSON().repository_id, 41003);
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authorization_url: 'http://127.0.0.1:8090/repository-authorize' } }) });
    });
    await accountMenu.goto(`${baseURL}/account/github`);
    await accountMenu.getByRole('heading', { name: 'Import a project from GitHub' }).waitFor();
    await accountMenu.getByText('test-owner/fixture').waitFor();
    assert.equal(await accountMenu.getByRole('button', { name: 'Connect GitHub repository App' }).count(), 0, 'connected installation should not show a duplicate connect action');
    await accountMenu.getByRole('link', { name: 'Manage repository access on GitHub' }).waitFor();
    await accountMenu.getByRole('button', { name: 'Refresh repositories' }).click();
    await accountMenu.waitForURL('http://127.0.0.1:8090/repository-authorize');
    await accountMenu.goto(`${baseURL}/account/github`);
    await accountMenu.getByText('test-owner/fixture').waitFor();
    await accountMenu.getByRole('button', { name: 'Import privately' }).click();
    await accountMenu.waitForURL('http://127.0.0.1:8090/repository-authorize');
    await accountMenu.goto(`${baseURL}/account/github`);
    let revokePrompt='';
    accountMenu.on('dialog', async dialog => { revokePrompt=dialog.message(); await dialog.accept(); });
    await accountMenu.getByRole('button', { name: 'Revoke PeerGit connection' }).click();
    await accountMenu.getByText('Connection revoked. Its imports are hidden').waitFor();
    assert.match(revokePrompt,/recoverable for 30 days/);
    assert.match(revokePrompt,/Imported fixture/);
    await accountMenu.getByRole('button', { name: 'Restore privately' }).click();
    await accountMenu.getByText('Imported fixture restored privately.').waitFor();
    await accountMenu.close();
    console.log('Discovery-only homepage, accessible account menu, and private GitHub import initiation passed');

    const profile = await browser.newPage({ viewport: { width: 390, height: 844 } });
    await profile.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'profile-csrf', user: { id: 'profile-user', display_name: 'Profile Student', college_id: '', campus_status: 'unverified', account_type: 'unverified' } } }) }));
    await profile.route('**/api/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'profile-user', email: 'profile@users.noreply.test', display_name: 'Profile Student', college_id: '', account_type: 'unverified', campus_status: 'unverified', profile: {}, skills: [], consents: ['terms', 'privacy'] } }) }));
    await profile.route('**/api/v1/media/intents', async route => {
      assert.equal(route.request().postDataJSON().visibility, 'private', 'unverified profile images stay private');
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: 'profile-intent' } }) });
    });
    await profile.route('**/api/v1/media/intents/profile-intent/content', async route => {
      assert.match(route.request().headers()['content-type'] ?? '', /^multipart\/form-data;/);
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: 'profile-image' } }) });
    });
    await profile.goto(`${baseURL}/account/profile`);
    await profile.getByRole('heading', { name: 'Profile Student' }).waitFor();
    await profile.getByLabel('Profile image (PNG/JPEG, up to 5 MiB)').setInputFiles({ name: 'avatar.png', mimeType: 'image/png', buffer: Buffer.from('synthetic fixture bytes') });
    await profile.getByText('Image uploaded and safely re-encoded. Save your profile to use it.').waitFor();
    await profile.getByRole('link', { name: 'View your uploaded image' }).waitFor();
    assert(await profile.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'profile image flow overflowed on mobile');
    await profile.close();
    console.log('Unverified profile image upload path passed at mobile width');

    const projectPage = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    projectPage.on('pageerror', error => console.error(`Project detail browser error: ${error.message}`));
    projectPage.on('console', message => { if (message.type() === 'error') console.error(`Project detail console error: ${message.text()}`); });
    await projectPage.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'project-csrf', user: { display_name: 'Test Owner', college_id: 'campus-1', terms_accepted: true, privacy_accepted: true } } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/repository', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { repository: { id: 'binding-1', full_name: 'test-owner/imported-project', html_url: 'https://github.com/test-owner/imported-project', description: 'Synthetic repo', default_branch: 'main', topics: [], languages: {}, readme_markdown: '', access_state: 'active', sync_health: 'current', limitations: 'LFS and submodule content is not captured.' }, contributions: [{id:'commit-1',kind:'commit',canonical_id:'git:sha1:0123456789abcdef0123456789abcdef01234567',title:'Initial commit',summary:'Initial commit',author_name:'Test Owner',author_login:'test-owner'}, {id:'issue-19',kind:'issue',number:19,state:'open',title:'Example issue',summary:'Synchronized issue detail'}, {id:'pr-14',kind:'pull_request',number:14,state:'open',title:'Example pull request',summary:'Synchronized pull request detail'}], snapshots: [] } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/repository/branches', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { default_branch:'main',items:[{name:'main',commit:{sha:'0123456789abcdef0123456789abcdef01234567'}}] } }) }));
    const fixtureSHA = '0123456789abcdef0123456789abcdef01234567';
    let treeRateLimited = true;
    await projectPage.route('**/api/v1/projects/imported-project/repository/tree**', route => {
      if (treeRateLimited) {
        treeRateLimited = false;
        return route.fulfill({ status: 429, contentType: 'application/json', body: JSON.stringify({ error: { message: 'GitHub rate limit reached' } }) });
      }
      const query = new URL(route.request().url()).searchParams;
      assert.equal(query.get('sha') || query.get('ref'), query.has('sha') ? fixtureSHA : 'main');
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { ref:'main',sha:fixtureSHA,tree:[{path:'README.md',type:'blob',size:5,sha:fixtureSHA}] } }) });
    });
    await projectPage.route('**/api/v1/projects/imported-project/repository/history**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [{ sha: fixtureSHA, commit: { message: 'Initial commit', author: { name: 'Test Owner', date: '2026-11-25T12:00:00Z' } } }] } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/repository/file**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { path:'README.md',ref:'0123456789abcdef0123456789abcdef01234567',content:'# Hi' } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/repository/commit**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { sha:'0123456789abcdef0123456789abcdef01234567',files:[{filename:'README.md',status:'modified',patch:'+hello'}] } }) }));
    await projectPage.route('**/api/v1/projects/imported-project/applications', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [] } }) }));
    const mockProject = { id: 'imported-project', title: 'Imported project', summary: 'Original summary', visibility: 'private', lifecycle: 'draft', project_type: 'open_source', recruiting: false, skills: [], version: 1 };
    let projectAccess = true;
    await projectPage.route('**/api/v1/projects/imported-project', async route => {
      if (!projectAccess) {
        await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: { message: 'Project unavailable' } }) });
        return;
      }
      if (route.request().method() === 'PATCH') {
        const body = route.request().postDataJSON();
        assert.equal(body.summary, 'Updated summary');
        Object.assign(mockProject, { summary: body.summary, visibility: body.visibility, lifecycle: body.lifecycle, version: mockProject.version + 1 });
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: mockProject.id, version: mockProject.version } }) });
      } else {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { project: { ...mockProject }, description: 'Original description', viewer_role: 'owner', members: [{ user_id: 'owner-1', display_name: 'Test Owner', role: 'owner' }] } }) });
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
    await projectPage.getByRole('alert').filter({ hasText: 'GitHub rate limit reached' }).waitFor();
    await projectPage.getByRole('button', { name: 'Retry' }).click();
    await projectPage.getByRole('button', { name: 'README.md' }).click();
    await projectPage.getByText('# Hi').waitFor();
    assert.equal(new URL(projectPage.url()).searchParams.get('path'), 'README.md');
    await projectPage.goBack();
    await projectPage.getByRole('button', { name: 'README.md' }).waitFor();
    await projectPage.goForward();
    await projectPage.getByText('# Hi').waitFor();
    await projectPage.getByRole('link', { name: 'Activity' }).click();
    await projectPage.getByRole('button', { name: 'Initial commit' }).click();
    await projectPage.getByText('+hello').waitFor();
    assert.equal(new URL(projectPage.url()).searchParams.get('commit'), '0123456789abcdef0123456789abcdef01234567');
    await projectPage.getByRole('link', { name: 'Observed issues' }).click();
    await projectPage.getByRole('link', { name: /Example issue/ }).click();
    await projectPage.getByText('Synchronized issue detail').waitFor();
    assert.equal(new URL(projectPage.url()).searchParams.get('item'), 'issue-19');
    await projectPage.getByRole('link', { name: 'Pull requests', exact: true }).click();
    await projectPage.getByRole('link', { name: /Example pull request/ }).click();
    await projectPage.getByText('Synchronized pull request detail').waitFor();
    await projectPage.getByRole('link', { name: 'Settings' }).click();
    await projectPage.getByRole('textbox', { name: 'Summary' }).fill('Updated summary');
    await projectPage.getByRole('button', { name: 'Save details' }).click();
    await projectPage.getByText('Project details saved.').waitFor();
    await projectPage.getByRole('button', { name: 'Publish to campus' }).click();
    await projectPage.getByText(/Project published to your campus/).waitFor();
    await projectPage.getByRole('button', { name: 'Make private' }).click();
    await projectPage.getByText('Project made private.').waitFor();
    await projectPage.getByRole('link', { name: 'Open roles' }).click();
    await projectPage.getByRole('textbox', { name: 'Role title' }).fill('Rust contributor');
    await projectPage.getByRole('textbox', { name: 'What will the member work on?' }).fill('Improve the project documentation.');
    await projectPage.getByRole('button', { name: 'Create opening' }).click();
    await projectPage.getByText('Team opening created.').waitFor();
    await projectPage.goto(`${baseURL}/projects/imported-project?tab=code&ref=old-ref&sha=${fixtureSHA}&path=README.md`);
    await projectPage.getByText('# Hi').waitFor();
    await projectPage.getByText('Pinned revision', { exact: false }).waitFor();
    projectAccess = false;
    await projectPage.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
    await projectPage.getByRole('alert').filter({ hasText: 'Project unavailable' }).waitFor();
    assert.equal(await projectPage.getByText('# Hi').count(), 0);
    await projectPage.close();
    console.log('Repository refresh, project code/activity tabs, publication reversal, and recruiting controls passed');

    const team = { members: [{ user_id: 'owner', display_name: 'Test Owner', role: 'owner' }], applications: [] };
    const teamProject = { id: 'team-project', title: 'Campus robotics', summary: 'Build a campus robot', visibility: 'campus', lifecycle: 'active', project_type: 'open_source', recruiting: true, skills: [], version: 1 };
    const teamRole = { id: 'team-role', title: 'Prototype builder', description: 'Build and test the sensor module.', openings: 1, status: 'open', version: 1, difficulty: 'beginner', good_first_task: true };
    const mountStudent = async (name, userID, owner) => {
      const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
      await page.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: `${userID}-csrf`, user: { id: userID, display_name: name, college_id: 'campus-1', campus_status: 'verified', terms_accepted: true, privacy_accepted: true } } }) }));
      await page.route('**/api/v1/projects/team-project', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { project: teamProject, description: '', viewer_role: owner ? 'owner' : (team.members.some(member => member.user_id === userID) ? 'member' : ''), members: team.members } }) }));
      await page.route('**/api/v1/projects/team-project/roles', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: [teamRole] } }) }));
      await page.route('**/api/v1/projects/team-project/applications', async route => {
        if (route.request().method() === 'POST') {
          team.applications.push({ id: 'team-application', role_id: teamRole.id, applicant_name: name, message: route.request().postDataJSON().message, status: 'pending', version: 1 });
          await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: 'team-application' } }) });
        } else {
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { items: owner ? team.applications : [] } }) });
        }
      });
      await page.route('**/api/v1/projects/team-project/applications/team-application/decision', async route => {
        assert.equal(route.request().postDataJSON().decision, 'accepted');
        team.applications[0].status = 'accepted';
        team.members.push({ user_id: 'applicant', display_name: 'Test Applicant', role: 'member' });
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { status: 'accepted' } }) });
      });
      await page.goto(`${baseURL}/projects/team-project?tab=open-roles`);
      await page.getByRole('heading', { name: 'Campus robotics' }).waitFor();
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `${name} project page overflowed on mobile`);
      return page;
    };
    const applicant = await mountStudent('Test Applicant', 'applicant', false);
    await applicant.getByRole('textbox', { name: 'Why are you interested?' }).fill('I can build and test the sensor module.');
    await applicant.getByRole('button', { name: 'Apply' }).click();
    await applicant.getByText('Application sent.').waitFor();
    const owner = await mountStudent('Test Owner', 'owner', true);
    await owner.getByRole('heading', { name: 'Test Applicant · pending' }).waitFor();
    await owner.getByRole('button', { name: 'Accept' }).click();
    await owner.getByText('Application accepted.').waitFor();
    await applicant.reload();
    await applicant.getByText('Test Applicant · member').waitFor();
    await applicant.close();
    await owner.close();
    console.log('Two-student project application and acceptance passed at mobile width');

    const onboarding = await browser.newPage({ viewport: { width: 390, height: 844 } });
    let confirmationPosts = 0;
    let challengeRequested = false;
    await onboarding.route('**/api/v1/session', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { authenticated: true, csrf_token: 'browser-test-csrf' } }) }));
    await onboarding.route('**/api/v1/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { id: 'browser-test-user', college_id: '', email: 'student@users.noreply.test', display_name: 'Browser Test Student', account_type: 'unverified', profile: {}, skills: [], consents: ['terms', 'privacy'], campus_admin: false, mfa_enabled: false } }) }));
    await onboarding.route('**/api/v1/me/onboarding', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: { account_type: 'unverified', campus_id: '', campus_verified: false, ...(challengeRequested ? { pending_challenge: { challenge_id: 'browser-test-challenge', email: 'st***@local.edu', expires_at: new Date(Date.now() + 600_000).toISOString() } } : {}) } }) }));
    await onboarding.route('**/api/v1/me/campus-verification/challenges', async route => {
      assert.equal(route.request().postDataJSON().campus_email, 'student-a1b2c3@local.edu');
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
    await onboarding.getByRole('textbox', { name: 'Campus email', exact: true }).fill('student-a1b2c3@local.edu');
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
    const liveCampusEmail = `student-${randomBytes(3).toString('hex')}@local.edu`;
    await live.goto(`${baseURL}/signup`);
    await live.getByRole('link', { name: /Continue with GitHub/ }).click();
    await live.getByRole('heading', { name: 'Set up your PeerGit account' }).waitFor();
    await live.getByRole('heading', { name: /Test Student/ }).waitFor();
    assert.equal(await live.locator('.account-panel [role="alert"]').count(), 0, 'a new account with no skills should load without a profile error');
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
