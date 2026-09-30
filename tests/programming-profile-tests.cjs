// Runs inside the pinned VS Code Extension Host as the student: activates
// every seeded extension and records the outcome for the VM test script.
const fs = require('node:fs');
const vscode = require('vscode');

exports.run = async () => {
  const ids = JSON.parse(process.env.NIXORIUM_TEST_EXTENSIONS);
  const result = {};
  for (const id of ids) {
    const extension = vscode.extensions.getExtension(id);
    if (!extension) { result[id] = 'missing'; continue; }
    try {
      await Promise.race([
        extension.activate(),
        new Promise((_, reject) => setTimeout(() => reject(new Error('activation timed out')), 90000)),
      ]);
      result[id] = extension.isActive ? `active ${extension.packageJSON.version}` : 'inactive';
    } catch (error) {
      result[id] = 'error: ' + String(error && error.message || error).slice(0, 300);
    }
  }
  const settings = vscode.workspace.getConfiguration();
  const proof = {
    result,
    telemetry: settings.get('telemetry.telemetryLevel'),
    theme: settings.get('workbench.colorTheme'),
    updateMode: settings.get('update.mode'),
    autoUpdate: settings.get('extensions.autoUpdate'),
    all: vscode.extensions.all.filter(e => !e.packageJSON.isBuiltin).map(e => e.id).sort(),
  };
  fs.writeFileSync('/home/student/editor-proof.json', JSON.stringify(proof, null, 1));
};
