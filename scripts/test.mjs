#!/usr/bin/env node
import { spawn, spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createWriteStream, existsSync, readdirSync, readFileSync, rmSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const windows = process.platform === 'win32';
const npm = windows ? 'npm.cmd' : 'npm';
const npmCLI = join(dirname(process.execPath), 'node_modules', 'npm', 'bin', 'npm-cli.js');
const composeFile = 'deploy/compose/local.yml';
const localDatabase = 'postgres://peergit:local-development-only@127.0.0.1:5432/peergit?sslmode=disable';
const env = { ...process.env, CI: 'true', DATABASE_URL: localDatabase, TEST_DATABASE_URL: localDatabase };
const temporary = mkdtempSync(join(tmpdir(), 'peergit-test-'));
const webLogPath = join(temporary, 'web.log');
let webProcess;
let composeServicesBefore = [];
let composeStateCaptured = false;

function display(command, args) {
  return [command, ...args].join(' ');
}

function run(command, args, options = {}) {
  const cwd = options.cwd ?? root;
  console.log(`\n> ${display(command, args)}`);
  const actualCommand = command === 'npm.cmd' ? process.execPath : command;
  const actualArgs = command === 'npm.cmd' ? [npmCLI, ...args] : args;
  return new Promise((resolveRun, rejectRun) => {
    const child = spawn(actualCommand, actualArgs, {
      cwd,
      env: options.env ?? env,
      stdio: options.capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
    });
    let stdout = '';
    let stderr = '';
    if (options.capture) {
      child.stdout.setEncoding('utf8').on('data', (chunk) => { stdout += chunk; });
      child.stderr.setEncoding('utf8').on('data', (chunk) => { stderr += chunk; });
    }
    child.once('error', rejectRun);
    child.once('close', (code) => {
      if (code !== 0) {
        rejectRun(new Error(`${display(command, args)} exited with code ${code}${stderr ? `\n${stderr}` : ''}`));
      } else {
        resolveRun(options.capture ? stdout.trim() : '');
      }
    });
  });
}

function capture(command, args, cwd = root) {
  const result = spawnSync(command, args, {
    cwd,
    env,
    encoding: 'utf8',
    shell: windows && command === 'npm.cmd',
    maxBuffer: 10 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${display(command, args)} exited with code ${result.status}\n${result.stderr}`);
  }
  return result.stdout.trim();
}

function listGoFiles(directory, files = []) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.isDirectory() && ['.git', 'node_modules', 'vendor'].includes(entry.name)) continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) listGoFiles(path, files);
    else if (entry.isFile() && entry.name.endsWith('.go')) files.push(path);
  }
  return files;
}

async function staticcheckPath() {
  const goBin = capture('go', ['env', 'GOBIN']) || join(capture('go', ['env', 'GOPATH']).split(delimiter)[0], 'bin');
  const executable = join(goBin, windows ? 'staticcheck.exe' : 'staticcheck');
  if (!existsSync(executable)) {
    await run('go', ['install', 'honnef.co/go/tools/cmd/staticcheck@2026.2.1']);
  }
  if (!existsSync(executable)) throw new Error(`staticcheck was not found at ${executable}`);
  return executable;
}

async function installNodePackages(directory) {
  const nodeModules = join(directory, 'node_modules');
  const marker = join(nodeModules, '.peergit-lock-hash');
  const lockHash = createHash('sha256')
    .update(readFileSync(join(directory, 'package.json')))
    .update(readFileSync(join(directory, 'package-lock.json')))
    .digest('hex');
  const localInstallMatches = existsSync(nodeModules) && existsSync(marker) && readFileSync(marker, 'utf8') === lockHash;
  if (process.env.CI === 'true' || !localInstallMatches) {
    await run(npm, ['ci'], { cwd: directory });
    writeFileSync(marker, lockHash);
  } else {
    console.log(`\n> Reusing npm dependencies in ${nodeModules}; package files match the saved lock hash.`);
  }
}

function installedChromiumPath(directory) {
  const result = spawnSync(process.execPath, ['-e', "process.stdout.write(require('playwright').chromium.executablePath())"], {
    cwd: directory,
    env,
    encoding: 'utf8',
  });
  if (result.status !== 0) throw new Error(`Could not locate Playwright Chromium: ${result.stderr}`);
  return result.stdout.trim();
}

function startWeb() {
  const url = 'http://127.0.0.1:3000/';
  const alreadyRunning = spawnSync(process.execPath, ['-e', `fetch(${JSON.stringify(url)}).then(()=>process.exit(0)).catch(()=>process.exit(1))`], { encoding: 'utf8' });
  if (alreadyRunning.status === 0) throw new Error('Port 3000 already serves a page. Stop that web server and rerun the test script.');

  const log = createWriteStream(webLogPath);
  const nextCLI = join(root, 'apps', 'web', 'node_modules', 'next', 'dist', 'bin', 'next');
  webProcess = spawn(process.execPath, [nextCLI, 'start', '--hostname', '127.0.0.1', '--port', '3000'], {
    cwd: join(root, 'apps', 'web'),
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  webProcess.stdout.pipe(log);
  webProcess.stderr.pipe(log);
  return log;
}

async function waitForWeb() {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (webProcess.exitCode !== null) throw new Error(`Next.js exited early with code ${webProcess.exitCode}`);
    try {
      const response = await fetch('http://127.0.0.1:3000/');
      if (response.ok) return;
    } catch { /* The production server is still starting. */ }
    await new Promise((resolveWait) => setTimeout(resolveWait, 1000));
  }
  throw new Error('Next.js did not become ready on http://127.0.0.1:3000 within 30 seconds');
}

async function stopWeb() {
  if (!webProcess || webProcess.exitCode !== null) return;
  webProcess.kill('SIGTERM');
  await Promise.race([
    new Promise((resolveWait) => webProcess.once('close', resolveWait)),
    new Promise((resolveWait) => setTimeout(resolveWait, 5000)),
  ]);
  if (webProcess.exitCode === null) webProcess.kill('SIGKILL');
}

async function cleanupCompose() {
  if (!composeStateCaptured) return;
  const now = capture('docker', ['compose', '-f', composeFile, 'ps', '--services', '--status', 'running']);
  const initial = new Set(composeServicesBefore);
  const startedHere = now.split(/\r?\n/).filter((service) => service && !initial.has(service));
  if (process.env.CI && composeServicesBefore.length === 0) {
    await run('docker', ['compose', '-f', composeFile, 'down']);
  } else if (startedHere.length > 0) {
    await run('docker', ['compose', '-f', composeFile, 'stop', ...startedHere]);
  }
}

let webLog;
try {
  console.log('PeerGit CI-equivalent checks');

  const goFiles = listGoFiles(root);
  if (goFiles.length === 0) throw new Error('No Go source files were found');
  const unformatted = capture('gofmt', ['-l', ...goFiles]);
  if (unformatted) throw new Error(`Go files need formatting:\n${unformatted}`);

  await staticcheckPath();
  composeServicesBefore = capture('docker', ['compose', '-f', composeFile, 'ps', '--services', '--status', 'running'])
    .split(/\r?\n/).filter(Boolean);
  composeStateCaptured = true;
  await run('docker', ['compose', '-f', composeFile, 'up', '-d', '--wait']);

  await run('go', ['run', './cmd/migrate']);
  await run('go', ['test', './...', '-count=1']);
  await run('go', ['vet', './...']);
  await run(await staticcheckPath(), ['./...']);

  const webDirectory = join(root, 'apps', 'web');
  await installNodePackages(webDirectory);
  await run(npm, ['run', 'typecheck'], { cwd: webDirectory });
  await run(npm, ['run', 'build'], { cwd: webDirectory });

  const e2eDirectory = join(root, 'tests', 'e2e');
  await installNodePackages(e2eDirectory);
  const browserPath = installedChromiumPath(e2eDirectory);
  if (process.env.CI === 'true' || !existsSync(browserPath)) {
    const playwrightArgs = ['exec', '--', 'playwright', 'install'];
    if (process.platform === 'linux') playwrightArgs.push('--with-deps');
    playwrightArgs.push('chromium');
    await run(npm, playwrightArgs, { cwd: e2eDirectory });
  } else {
    console.log(`\n> Reusing Playwright Chromium at ${browserPath}`);
  }

  webLog = startWeb();
  await waitForWeb();
  await run(npm, ['test'], { cwd: e2eDirectory });
  await run('docker', ['compose', '-f', composeFile, 'config', '--quiet']);
  await run('git', ['diff', '--check']);
  console.log('\nAll PeerGit checks passed.');
} catch (error) {
  console.error(`\nTest script failed: ${error.message}`);
  if (existsSync(webLogPath)) {
    const logs = readFileSync(webLogPath, 'utf8');
    if (logs) console.error(`\nNext.js log:\n${logs}`);
  }
  process.exitCode = 1;
} finally {
  await stopWeb().catch((error) => console.error(`Could not stop Next.js cleanly: ${error.message}`));
  try {
    await cleanupCompose();
  } catch (error) {
    console.error(`Could not clean up test services: ${error.message}`);
    process.exitCode = 1;
  }
  rmSync(temporary, { recursive: true, force: true });
}
