// Runs inside the pinned VS Code Extension Host, not a stand-alone Node mock.
// https://code.visualstudio.com/api/working-with-extensions/testing-extension
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const vscode = require('vscode');

function localPage() {
  return new Promise((resolve, reject) => {
    const request = http.get('http://127.0.0.1:5500/index.html', response => {
      let body = '';
      response.setEncoding('utf8');
      response.on('data', chunk => { body += chunk; });
      response.on('end', () => resolve({ status: response.statusCode, body }));
    });
    request.setTimeout(500, () => request.destroy(new Error('local server timed out')));
    request.on('error', reject);
  });
}

exports.run = async () => {
  assert.equal(process.getuid(), 2000);
  assert.equal(vscode.version, process.env.NIXORIUM_TEST_EDITOR_VERSION);
  const settings = vscode.workspace.getConfiguration();
  assert.equal(settings.get('editor.fontSize'), Number(process.env.NIXORIUM_TEST_FONT_SIZE));
  assert.equal(settings.get('extensions.autoUpdate'), false);
  assert.equal(settings.get('extensions.autoCheckUpdates'), false);
  assert.equal(settings.get('update.mode'), 'none');
  await settings.update('editor.fontSize', 21, vscode.ConfigurationTarget.Global);
  assert.equal(vscode.workspace.getConfiguration().get('editor.fontSize'), 21);

  const extension = vscode.extensions.getExtension('ritwickdey.liveserver');
  const expected = process.env.NIXORIUM_TEST_EXPECT_EXTENSION === '1';
  let served = false;
  if (expected) {
    assert.ok(extension, 'the preloaded extension must be discovered');
    assert.equal(extension.packageJSON.version, process.env.NIXORIUM_TEST_EXTENSION_VERSION);
    assert.equal(fs.realpathSync(extension.extensionPath), process.env.NIXORIUM_TEST_EXTENSION_PATH);
    await settings.update('liveServer.settings.NoBrowser', true, vscode.ConfigurationTarget.Global);
    await settings.update('liveServer.settings.port', 5500, vscode.ConfigurationTarget.Global);
    await extension.activate();
    assert.equal(extension.isActive, true);
    assert.ok((await vscode.commands.getCommands()).includes('extension.liveServer.goOnline'));
    await vscode.commands.executeCommand('extension.liveServer.goOnline', vscode.Uri.file('/home/student/editor-project/index.html'));
    const deadline = Date.now() + 15000;
    while (Date.now() < deadline && !served) {
      try {
        const page = await localPage();
        served = page.status === 200 && page.body.includes('nixorium-editor-proof');
      } catch (_) { /* Poll only the local server readiness, with a deadline. */ }
      if (!served) await new Promise(resolve => setTimeout(resolve, 100));
    }
    assert.ok(served, 'the activated plugin must serve the local test page');
    await vscode.commands.executeCommand('extension.liveServer.goOffline');
  } else {
    assert.equal(extension, undefined, 'a removed extension must not reappear after boot reset');
  }
  fs.writeFileSync('/home/student/editor-proof.json', JSON.stringify({
    student: true,
    editorVersion: vscode.version,
    extensionVersion: extension?.packageJSON.version ?? null,
    extensionActive: extension?.isActive ?? false,
    served,
    updatesDisabled: true,
    settingsEditable: true,
  }));
};
