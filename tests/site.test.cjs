const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {spawnSync} = require('node:child_process');
const os = require('node:os');
const path = require('node:path');
const {installation} = require('../site/app.js');

test('downloads and commands match Linux and Windows release targets', () => {
  for (const arch of ['amd64', 'arm64']) {
    const linux = installation('linux', arch);
    assert.ok(linux.url.endsWith(`gobook-v0.8.1-linux-${arch}.tar.gz`));
    assert.ok(linux.command.includes(`cd gobook-v0.8.1-linux-${arch}\n./install.sh`));
    assert.ok(linux.command.includes('sha256sum -c '));
    const windows = installation('windows', arch);
    assert.ok(windows.url.endsWith(`gobook-v0.8.1-windows-${arch}.zip`));
    assert.ok(windows.command.includes(`Expand-Archive .\\gobook-v0.8.1-windows-${arch}.zip`));
    assert.ok(windows.command.includes('.\\Install-GoBook.ps1'));
  }
});
test('macOS offers the compiler-based path instead of a nonexistent archive', () => {
  const setup = installation('macos', 'arm64');
  assert.equal(setup.url, null);
  assert.equal(setup.command, 'go run github.com/Alifarid0011/gobook@v0.8.1');
});
test('unknown targets do not produce a download or executable command', () => {
  assert.throws(() => installation('linux', '../bad'));
  assert.throws(() => installation('other', 'amd64'));
});
test('first-steps download is a valid notebook whose cells run using only Go', () => {
  const notebook = JSON.parse(fs.readFileSync(path.join(__dirname, '../site/examples/first-steps.gobook.json'), 'utf8'));
  assert.equal(notebook.version, 1);
  assert.deepEqual(notebook.imports, ['fmt']);
  assert.ok(notebook.cells.length >= 2);
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'gobook-site-'));
  try {
    fs.writeFileSync(path.join(temp, 'main.go'), 'package main\nimport "fmt"\nfunc main(){\n' + notebook.cells.map(c => c.code).join('\n') + '\n}\n');
    const run = spawnSync('go', ['run', path.join(temp, 'main.go')], {encoding:'utf8'});
    assert.equal(run.status, 0, run.stderr);
    assert.match(run.stdout, /Total: 60/);
    assert.match(run.stdout, /Average: 20/);
  } finally {fs.rmSync(temp, {recursive:true, force:true});}
});
