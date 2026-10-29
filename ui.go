package main

const appJS = `
'use strict';
const $ = id => document.getElementById(id);
let token = new URL(location.href).searchParams.get('token') || sessionStorage.getItem('gobook-token') || '';
sessionStorage.setItem('gobook-token', token);
history.replaceState(null, '', '/');
let debugging = false, busy = false, changed = false, loadingExample = false, lastText = '', inputFiles = [], lastExperiment = null, toastTimer, runTimer;
const editorViews = new WeakMap();
let preferences = {theme:'paper',font:14,indent:4,wrap:false};
try { const saved = JSON.parse(localStorage.getItem('gobook.preferences.v1') || '{}'); preferences = {...preferences,...saved}; } catch {}
function applyPreferences() {
 if (!['paper','dark'].includes(preferences.theme)) preferences.theme = 'paper';
 if (![12,14,16,18,20].includes(preferences.font)) preferences.font = 14;
 if (![2,4,8].includes(preferences.indent)) preferences.indent = 4;
 preferences.wrap = preferences.wrap === true;
 document.documentElement.dataset.theme = preferences.theme; document.body.dataset.wrap = String(preferences.wrap);
 document.documentElement.style.setProperty('--editor-size', preferences.font+'px'); document.documentElement.style.setProperty('--editor-indent', preferences.indent);
 for (const key of ['theme','font','indent','wrap']) $('pref-'+key).value = String(preferences[key]);
 document.querySelectorAll('.cell textarea').forEach(editor => { editor.style.tabSize = preferences.indent; editor.wrap = preferences.wrap ? 'soft' : 'off'; editorViews.get(editor)?.refresh(); });
 try { localStorage.setItem('gobook.preferences.v1',JSON.stringify(preferences)); } catch {}
}
$('appearance').onclick = () => { $('preferences').hidden = !$('preferences').hidden; $('appearance').setAttribute('aria-expanded',String(!$('preferences').hidden)); };
for (const key of ['theme','font','indent','wrap']) $('pref-'+key).onchange = () => { preferences[key] = key === 'font' || key === 'indent' ? Number($('pref-'+key).value) : key === 'wrap' ? $('pref-wrap').value === 'true' : $('pref-theme').value; applyPreferences(); };
applyPreferences();
function setView(view) {
 document.body.dataset.view = view;
 document.querySelectorAll('button[data-view]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.view === view)));
}
document.querySelectorAll('button[data-view]').forEach(button => button.onclick = () => setView(button.dataset.view));
document.querySelectorAll('.side-link[href]').forEach(link => link.addEventListener('click', () => setView(link.hash === '#results' ? 'results' : 'both')));
new ResizeObserver(entries => document.documentElement.style.setProperty('--toolbar-height', entries[0].target.offsetHeight + 'px')).observe(document.querySelector('.topbar'));
function download(content, name, mime) {
 const url = URL.createObjectURL(new Blob([content], { type: mime })); const a = document.createElement('a');
 a.href = url; a.download = name; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}
$('copy-results').onclick = async () => {
 try { await navigator.clipboard.writeText(lastText); notify('Results copied.'); }
 catch { notify('Clipboard unavailable. Use Download text instead.'); }
};
$('download-results').onclick = () => download(lastText, 'gobook-results.txt', 'text/plain;charset=utf-8');
$('close-figure').onclick = () => $('figure-dialog').close();
function exportText(text = '') {
 lastText = text; $('copy-results').disabled = !text; $('download-results').disabled = !text;
}
function setExperiment(experiment = null) {
 lastExperiment = experiment; $('save-experiment').disabled = !experiment;
 const provenance = experiment && experiment.result && experiment.result.provenance;
 $('run-details').hidden = !provenance; $('provenance-output').textContent = provenance ? JSON.stringify(provenance, null, 2) : '';
}
$('save-experiment').onclick = () => {
 if (lastExperiment) download(JSON.stringify(lastExperiment, null, 2), 'experiment.gobook.json', 'application/json');
 notify('Experiment downloaded: source snapshot, inputs, results and provenance. Later edits are saved separately with Save notebook.');
};
function showFiles() {
 $('input-files').replaceChildren();
 inputFiles.forEach((file, i) => {
  const row = document.createElement('li'); const label = document.createElement('span'); label.textContent = file.name;
  const remove = document.createElement('button'); remove.textContent = 'Remove input'; remove.disabled = busy || debugging || loadingExample;
  remove.onclick = () => { inputFiles.splice(i, 1); showFiles(); markChanged(); }; row.append(label, remove); $('input-files').append(row);
 });
}
function attachFiles(files) {
 const merged = [...inputFiles, ...files];
 if (merged.length > 8 || new Set(merged.map(f => f.name)).size !== merged.length) throw Error('Use up to 8 uniquely named input files. Remove an existing input before replacing it.');
 let total = 0;
 for (const f of merged) {
  if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,58}\.(csv|json)$/.test(f.name)) throw Error('Use CSV or JSON filenames with letters, numbers, dashes and underscores before the extension.');
  total += atob(f.data).length;
 }
 if (total > 131072) throw Error('Input files exceed 128 KiB total.');
 inputFiles = merged; showFiles(); markChanged();
}
$('data-files').onchange = async () => {
 const files = [...$('data-files').files]; if (!files.length || busy || debugging || loadingExample) return;
 loadingExample = true; setBusy(false);
 try {
  if (files.some(f => f.size > 131072)) throw Error('Input files exceed 128 KiB.');
  const additions = await Promise.all(files.map(f => new Promise((resolve, reject) => {
   const reader = new FileReader(); reader.onload = () => resolve({ name: f.name, data: String(reader.result).split(',')[1] }); reader.onerror = () => reject(Error('Could not read ' + f.name)); reader.readAsDataURL(f);
  })));
  attachFiles(additions); notify('Inputs attached. Code has not been run.');
 } catch (e) { notify(e.message); }
 finally { loadingExample = false; $('data-files').value = ''; setBusy(false); }
};
const mac = /Mac|iPhone|iPad/.test(navigator.platform);
$('keyboard-shortcut').textContent = (mac ? '⌘' : 'Ctrl') + ' Enter';
document.querySelector('.shortcut').textContent = mac ? '⌘ ↵' : 'Ctrl ↵';
function notify(message) {
 $('toast').textContent = message; $('toast').hidden = false;
 clearTimeout(toastTimer); toastTimer = setTimeout(() => $('toast').hidden = true, 3500);
}
function setStatus(message, state = '') { $('status').textContent = message; $('status').dataset.state = state; }
function updateConnection() {
 $('run').disabled = busy || debugging || loadingExample || !token;
 $('debug-start').disabled = busy || loadingExample || !token;
 document.querySelectorAll('.run-cell').forEach(button => button.disabled = busy || debugging || loadingExample || !token);
 $('kernel-dot').classList.toggle('connected', Boolean(token));
 $('kernel-label').textContent = token ? 'Local host configured' : 'Host not connected';
}
if (!token) { $('connection').hidden = false; setStatus('Connect a host to run'); }
updateConnection();
$('connect').onclick = () => { $('connection').hidden = !$('connection').hidden; if (!$('connection').hidden) $('capability').focus(); };
$('applytoken').onclick = () => {
 let entered = $('capability').value.trim();
 try { entered = new URL(entered).searchParams.get('token') || ''; } catch {}
 if (!/^[a-f0-9]{64}$/.test(entered)) { notify('Paste a complete GoBook private URL or a 64-character token.'); $('capability').focus(); return; }
 token = entered; sessionStorage.setItem('gobook-token', token); $('capability').value = ''; $('connection').hidden = true;
 updateConnection(); setStatus('Host configured · ready'); notify('Private host configured for this tab.');
};
$('capability').addEventListener('keydown', e => { if (e.key === 'Enter') $('applytoken').click(); });
let installPrompt;
addEventListener('beforeinstallprompt', e => { e.preventDefault(); installPrompt = e; $('install').hidden = false; });
$('install').onclick = async () => { if (installPrompt) { await installPrompt.prompt(); installPrompt = null; $('install').hidden = true; } };
addEventListener('appinstalled', () => { $('install').hidden = true; notify('GoBook installed.'); });
function markChanged() {
 changed = true; $('save-state').textContent = 'Unsaved changes';
 $('import-summary').textContent = $('imports').value || 'No imports';
 if ($('output').querySelector('pre,figure')) $('execution-note').textContent = 'Notebook edited · run again to refresh results.';
}
['title', 'note', 'imports', 'dependencies'].forEach(id => $(id).addEventListener('input', markChanged));
addEventListener('beforeunload', e => { if (changed) { e.preventDefault(); e.returnValue = ''; } });
function addCell(code = '', focus = false) {
 const section = document.createElement('article'); section.className = 'cell';
 const head = document.createElement('div'); head.className = 'cellhead';
 const label = document.createElement('span'); label.className = 'cell-label';
 const actions = document.createElement('div'); actions.className = 'cell-actions';
 const up = document.createElement('button'), down = document.createElement('button'), remove = document.createElement('button');
 const runCell = document.createElement('button'); runCell.type = 'button'; runCell.className = 'run-cell'; runCell.textContent = '▶ Run to here'; runCell.title = 'Start fresh and run all cells up to this one';
 runCell.onclick = () => runNotebook([...$('cells').children].indexOf(section) + 1);
 up.textContent = '↑'; up.title = 'Move cell up'; up.setAttribute('aria-label', 'Move cell up');
 down.textContent = '↓'; down.title = 'Move cell down'; down.setAttribute('aria-label', 'Move cell down');
 remove.textContent = 'Remove'; remove.title = 'Remove this cell';
 for (const button of [up, down, remove]) button.type = 'button';
 up.onclick = () => { if (section.previousElementSibling) { section.previousElementSibling.before(section); renumber(); markChanged(); } };
 down.onclick = () => { if (section.nextElementSibling) { section.nextElementSibling.after(section); renumber(); markChanged(); } };
 remove.onclick = () => {
  if (editor.value.trim() && !confirm('Remove this code cell? This cannot be undone.')) return;
  section.remove(); renumber(); markChanged();
 };
 actions.append(runCell, up, down, remove); head.append(label, actions);
 const wrap = document.createElement('div'); wrap.className = 'editor-wrap';
 const lines = document.createElement('div'); lines.className = 'line-numbers'; lines.setAttribute('aria-hidden', 'true');
 const editor = document.createElement('textarea'); editor.value = code; editor.spellcheck = false; editor.autocapitalize = 'off'; editor.autocomplete = 'off'; editor.wrap = 'off';
 function updateLines() { lines.textContent = Array.from({ length: editor.value.split('\n').length }, (_, i) => i + 1).join('\n'); lines.scrollTop = editor.scrollTop; }
 editor.addEventListener('input', () => { updateLines(); markChanged(); });
 editor.addEventListener('scroll', () => { lines.scrollTop = editor.scrollTop; });
 editor.addEventListener('keydown', e => {
  if (editor.readOnly) return;
  if (e.key === 'Enter' && e.shiftKey && !e.ctrlKey && !e.metaKey) { e.preventDefault(); runNotebook([...$('cells').children].indexOf(section) + 1); return; }
  if (e.key === 'Tab' && !e.shiftKey) { e.preventDefault(); editor.setRangeText(' '.repeat(preferences.indent), editor.selectionStart, editor.selectionEnd, 'end'); updateLines(); editorViews.get(editor)?.refresh(); markChanged(); }
 });
 const foot = document.createElement('div'); foot.className = 'cell-foot'; foot.textContent = 'Shift Enter · Run to here repeats earlier cells in a fresh process';
 wrap.append(lines, editor); section.append(head, wrap, foot); $('cells').append(section); updateLines(); renumber();
 editor.style.tabSize = preferences.indent; editor.wrap = preferences.wrap ? 'soft' : 'off';
 editorViews.set(editor, GoBookEditor.attach(wrap, editor));
 editor.style.height = Math.min(420, Math.max(128, (code.split('\n').length + 2) * 20)) + 'px';
 if (focus) { editor.focus(); section.scrollIntoView({ block: 'nearest' }); }
}
function renumber() {
 const cells = [...$('cells').children]; $('outline').replaceChildren();
 cells.forEach((cell, i) => {
  cell.id = 'cell-' + (i + 1); cell.querySelector('.cell-label').textContent = 'CELL ' + String(i + 1).padStart(2, '0');
  cell.querySelector('textarea').setAttribute('aria-label', 'Go code cell ' + (i + 1));
  cell.querySelector('[aria-label="Move cell up"]').disabled = i === 0; cell.querySelector('[aria-label="Move cell down"]').disabled = i === cells.length - 1;
  cell.querySelector('.run-cell').disabled = busy || debugging || loadingExample || !token;
  const item = document.createElement('button'); const number = document.createElement('em'); number.textContent = String(i + 1).padStart(2, '0');
  item.append(number, document.createTextNode('Code cell ' + (i + 1))); item.onclick = () => { setView('code'); cell.scrollIntoView({ block: 'center' }); cell.querySelector('textarea').focus({ preventScroll: true }); };
  $('outline').append(item);
 });
 $('cell-count').textContent = cells.length + ' code cell' + (cells.length === 1 ? '' : 's');
}
function notebook() {
 let dependencies;
 try { dependencies = JSON.parse($('dependencies').value || '{}'); } catch { throw Error('Dependencies must be a JSON object of module paths and exact versions.'); }
 if (!dependencies || Array.isArray(dependencies) || typeof dependencies !== 'object' || !Object.values(dependencies).every(v => typeof v === 'string')) throw Error('Dependencies must map module paths to version strings.');
 return { version: 1, title: $('title').value, note: $('note').value, imports: $('imports').value.split(',').map(s => s.trim()).filter(Boolean), dependencies, files: inputFiles, cells: [...document.querySelectorAll('.cell textarea')].map(t => ({ code: t.value })) };
}
function emptyOutput(title = 'Make room for a discovery.', note = 'Run your notebook to see its output and figures here.') {
 const empty = document.createElement('div'); empty.className = 'empty';
 const icon = document.createElement('span'); icon.className = 'empty-symbol'; icon.textContent = '↗';
 const strong = document.createElement('strong'); strong.textContent = title;
 const p = document.createElement('p'); p.textContent = note; empty.append(icon, strong, p); $('output').replaceChildren(empty);
}
function load(n) {
 if (!n || n.version !== 1 || typeof n.title !== 'string' || typeof n.note !== 'string' || !Array.isArray(n.imports) || !n.imports.every(x => typeof x === 'string') || !Array.isArray(n.cells) || !n.cells.every(c => c && typeof c.code === 'string')) throw Error('Invalid version 1 notebook');
 if (n.dependencies !== undefined && (!n.dependencies || Array.isArray(n.dependencies) || typeof n.dependencies !== 'object' || !Object.values(n.dependencies).every(v => typeof v === 'string'))) throw Error('Invalid notebook dependencies');
 if (n.files !== undefined && (!Array.isArray(n.files) || n.files.length > 8 || !n.files.every(f => f && typeof f.name === 'string' && typeof f.data === 'string'))) throw Error('Invalid notebook input files');
 const priorFiles = inputFiles; inputFiles = [];
 try { attachFiles(n.files || []); } catch (e) { inputFiles = priorFiles; showFiles(); throw e; }
 $('title').value = n.title; $('note').value = n.note; $('imports').value = n.imports.join(', '); $('import-summary').textContent = $('imports').value || 'No imports'; $('dependencies').value = JSON.stringify(n.dependencies || {}, null, 2);
 $('cells').replaceChildren(); n.cells.forEach(c => addCell(c.code)); renumber();
 exportText(); setExperiment(); setView('both'); $('starters').open = false;
 emptyOutput('A fresh perspective.', 'This notebook is loaded. Run all when you are ready.'); setStatus('Loaded · not executed');
 $('execution-note').textContent = 'Loaded files never run automatically.'; changed = false; $('save-state').textContent = 'Loaded from file';
}
$('add').onclick = () => { addCell('', true); markChanged(); };
$('new').onclick = () => {
 if (changed && !confirm('Start a new notebook? Save your changes first.')) return;
 load({ version: 1, title: 'Untitled exploration', note: '', imports: ['fmt'], cells: [{ code: 'fmt.Println("Hello, GoBook")' }] });
 $('save-state').textContent = 'Unsaved notebook'; setStatus(token ? 'Ready when you are' : 'Connect a host to run'); $('title').focus();
};
$('sidebar-new').onclick = () => $('new').click();
document.querySelectorAll('[data-example]').forEach(button => button.onclick = async () => {
 if (busy || debugging || loadingExample) return;
 if (changed && !confirm('Load this example? Download your current notebook first if you want to keep it.')) return;
 loadingExample = true; setBusy(false); $('example-status').textContent = 'Loading example…';
 try {
  const response = await fetch('/examples/' + button.dataset.example + '.gobook.json');
  if (!response.ok) throw Error('Could not load this example.');
  load(await response.json()); $('save-state').textContent = 'Unsaved example';
  $('example-status').textContent = 'Example loaded. Edit the sample in cell 1, then choose Run all. Nothing has run yet.';
  notify('Example loaded · not executed.');
  $('workspace').scrollIntoView({ block: 'start' });
 } catch (e) { $('example-status').textContent = e.message; notify(e.message); }
 finally { loadingExample = false; setBusy(false); }
});
$('save').onclick = () => {
 try { download(JSON.stringify(notebook(), null, 2), ($('title').value.trim().replace(/[^a-zA-Z0-9_-]+/g, '-').slice(0, 80) || 'notebook') + '.gobook.json', 'application/json'); } catch (e) { notify(e.message); return; }
 changed = false; $('save-state').textContent = 'Downloaded as JSON'; notify('Notebook downloaded as JSON.');
};
$('load').onclick = () => $('file').click();
$('file').onchange = async () => {
 const file = $('file').files[0]; if (!file) return;
 try {
  if (file.size > 16*1024*1024) throw Error('File exceeds 16 MiB');
  if (changed && !confirm('Open this notebook? Save your current changes first.')) return;
  loadingExample = true; setBusy(false);
  const value = JSON.parse(await file.text());
  if (value.format === 'gobook-experiment') {
   if (value.version !== 1) throw Error('Unsupported experiment version');
   validateResult(value.result); load(value.notebook); renderResult(value.result); setExperiment(value);
   setStatus('Imported results' + (value.notebook.runThrough ? ' · through cell ' + value.notebook.runThrough : '') + ' · not rerun'); $('execution-note').textContent = 'Historical results supplied by this file; not verified by this host. ' + (value.notebook.runThrough ? 'Use Run to here on cell ' + value.notebook.runThrough + ' to replay the saved prefix.' : 'Use Run all to replay the notebook.'); setView('results');
  } else { if (file.size > 262144) throw Error('Notebook exceeds 256 KiB'); load(value); }
  notify('Opened. Code has not been run.');
 }
 catch (e) { notify(e.message); } finally { $('file').value = ''; loadingExample = false; setBusy(false); }
};
function textOutput(label, text, error = false) {
 if (!text) return; const title = document.createElement('p'); title.className = 'output-label'; title.textContent = label;
 const pre = document.createElement('pre'); pre.textContent = text; if (error) pre.className = 'error'; $('output').append(title, pre);
}
function validateResult(result) {
 if (!result || typeof result !== 'object' || Array.isArray(result)) throw Error('Invalid experiment result');
 for (const key of ['stdout','stderr','error']) if (result[key] !== undefined && (typeof result[key] !== 'string' || result[key].length > 1048576)) throw Error('Invalid experiment output');
 const artifacts = result.artifacts || []; if (!Array.isArray(artifacts) || artifacts.length > 16) throw Error('Invalid experiment artifacts');
 let total = 0;
 for (const a of artifacts) {
  if (!a || typeof a.name !== 'string' || a.name.length > 255 || !['image/png','image/svg+xml','application/json','text/csv'].includes(a.mime) || typeof a.data !== 'string' || a.data.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(a.data)) throw Error('Invalid experiment artifact');
  total += a.data.length; if (total > 12*1024*1024) throw Error('Experiment artifacts exceed the import limit');
 }
}
function tablePreview(artifact) {
 if (artifact.mime !== 'application/json' || artifact.data.length > 524288) return null;
 try {
  const value = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(artifact.data),c => c.charCodeAt(0))));
  if (value.format !== 'gobook.table.v1' || !Array.isArray(value.columns) || value.columns.length > 50 || !value.columns.length || !value.columns.every(c => typeof c === 'string') || !Array.isArray(value.rows) || value.rows.length > 200 || !value.rows.every(row => Array.isArray(row) && row.length === value.columns.length && row.every(v => v === null || ['string','number','boolean'].includes(typeof v)))) return null;
  const container = document.createElement('div'); container.className = 'dataframe-view'; container.tabIndex = 0; container.setAttribute('role','region'); container.setAttribute('aria-label', typeof value.title === 'string' ? value.title : 'DataFrame preview');
  const table = document.createElement('table'); const caption = document.createElement('caption'); caption.textContent = (typeof value.title === 'string' ? value.title : 'DataFrame preview') + ' · '+value.rows.length+' rows shown × '+value.columns.length+' columns';
  const head = document.createElement('thead'); const header = document.createElement('tr'); const index = document.createElement('th'); index.textContent = '#'; header.append(index);
  for (const name of value.columns) { const cell = document.createElement('th'); cell.scope = 'col'; cell.textContent = name; header.append(cell); } head.append(header);
  const body = document.createElement('tbody'); value.rows.forEach((row,i) => { const tr = document.createElement('tr'); const number = document.createElement('th'); number.scope='row'; number.textContent=String(i); tr.append(number); for (const v of row) { const td = document.createElement('td'); td.textContent = v === null ? '—' : String(v); tr.append(td); } body.append(tr); });
  table.append(caption,head,body); container.append(table); return container;
 } catch { return null; }
}
function renderResult(result) {
 validateResult(result); $('output').replaceChildren();
  textOutput('STANDARD OUTPUT', result.stdout); textOutput('BUILD & RUN DIAGNOSTICS', result.stderr); textOutput('EXECUTION ERROR', result.error, true);
  if (result.truncated) textOutput('OUTPUT LIMIT', 'Output was truncated at 1 MiB.');
  exportText([result.stdout, result.stderr, result.error, result.truncated ? 'Output was truncated at 1 MiB.' : ''].filter(Boolean).join('\n'));
  for (const artifact of result.artifacts || []) {
   const figure = document.createElement('figure'); figure.className = 'artifact'; const caption = document.createElement('figcaption'); caption.textContent = artifact.name;
   const uri = 'data:' + artifact.mime + ';base64,' + artifact.data;
   const img = document.createElement('img'); img.alt = artifact.name; img.src = uri;
   const actions = document.createElement('div'); actions.className = 'figure-actions';
   const expand = document.createElement('button'); expand.textContent = 'Expand figure'; expand.onclick = () => { $('figure-title').textContent = artifact.name; $('expanded-figure').src = img.src; $('expanded-figure').alt = artifact.name; $('figure-dialog').showModal(); };
   const save = document.createElement('button'); save.type = 'button'; save.textContent = artifact.mime.startsWith('image/') ? '↓ Download figure' : '↓ Download file';
   save.onclick = event => { event.preventDefault(); download(Uint8Array.from(atob(artifact.data), c => c.charCodeAt(0)), artifact.name, artifact.mime); };
   if (artifact.mime.startsWith('image/')) { actions.append(expand, save); figure.append(caption, img, actions); }
   else {
    const use = document.createElement('button'); use.className = 'use-artifact'; use.textContent = 'Use as input'; use.onclick = () => { if (busy || debugging || loadingExample) { notify('Stop the current session before changing its inputs.'); return; } try { attachFiles([{name: artifact.name, data: artifact.data}]); notify('Attached to the current notebook. It will be available on the next run.'); } catch (e) { notify(e.message); } };
    const description = document.createElement('p'); description.className = 'file-description'; description.textContent = 'Generated data or model file'; actions.append(use, save); figure.append(caption, tablePreview(artifact) || description, actions);
   }
   $('output').append(figure);
  }
  if (!$('output').childNodes.length) emptyOutput('Quietly complete.', 'Your code finished successfully without producing output.');

}
function setBusy(value) {
 busy = value; document.body.classList.toggle('running', value); $('run-label').textContent = value ? 'Running' : 'Run all';
 for (const id of ['new', 'load', 'add', 'sidebar-new', 'data-files']) $(id).disabled = value || debugging || loadingExample;
 for (const id of ['title','note','imports','dependencies']) $(id).readOnly = loadingExample || debugging;
 document.querySelectorAll('.cell textarea').forEach(editor => editor.readOnly = loadingExample || debugging);
 document.querySelectorAll('[data-example]').forEach(button => button.disabled = value || debugging || loadingExample);
 document.querySelectorAll('.cell-actions button').forEach(b => b.disabled = value || debugging || loadingExample);
 document.querySelectorAll('.use-artifact').forEach(b => b.disabled = value || debugging || loadingExample);
 if (!value && !loadingExample && !debugging) renumber(); updateConnection(); showFiles(); $('output').setAttribute('aria-busy', String(value));
}
async function runNotebook(through = null) {
 if (busy || debugging || loadingExample || !token) return;
 let snapshot, sourceSnapshot; try { const n = notebook(); sourceSnapshot = JSON.stringify(n); if (through !== null) n.runThrough = through; snapshot = JSON.stringify(n); if (new TextEncoder().encode(snapshot).length > 262144) throw Error('Notebook request exceeds 256 KiB. Reduce code or attached inputs.'); } catch (e) { notify(e.message); return; }
 const start = performance.now(); setBusy(true);
 exportText(); setExperiment();
 emptyOutput('A thought in motion.', 'Compiling and running your Go notebook…');
 const elapsed = () => ((performance.now() - start) / 1000).toFixed(1);
 setStatus('Running · 0.0s'); runTimer = setInterval(() => setStatus('Running · ' + elapsed() + 's'), 1000);
 $('execution-note').textContent = 'Compiling, then executing in a fresh process.';
 try {
  const response = await fetch('/api/run', { method: 'POST', headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }, body: snapshot });
  if (!response.ok) throw Error(await response.text());
  const result = await response.json(); $('output').replaceChildren();
  renderResult(result); setExperiment({format:'gobook-experiment',version:1,notebook:JSON.parse(snapshot),result});
  clearInterval(runTimer); setStatus((result.error ? 'Failed' : 'Complete') + (through !== null ? ' · through cell ' + through : '') + ' · ' + elapsed() + 's', result.error ? 'error' : 'success');
  let edited = true; try { edited = JSON.stringify(notebook()) !== sourceSnapshot; } catch {}
  $('execution-note').textContent = edited ? 'Notebook edited during run · results reflect earlier code.' : (result.error ? 'Review the diagnostics, adjust, and try again.' : (through !== null ? (through === 1 ? 'Cell 1 executed.' : 'Cells 1–' + through + ' executed.') + ' Later cells were compiled but not executed.' : 'Fresh process finished. Keep exploring.'));
  if (matchMedia('(max-width: 1120px)').matches && !document.activeElement.matches('textarea,input')) { setView('results'); $('workspace').scrollIntoView({ block: 'start' }); }
 } catch (e) {
  clearInterval(runTimer); $('output').replaceChildren(); textOutput('REQUEST ERROR', e.message, true); exportText(e.message); setStatus('Could not run · ' + elapsed() + 's', 'error');
  $('execution-note').textContent = 'Check that your local GoBook host is running.';
 } finally { clearInterval(runTimer); setBusy(false); }
}
$('run').onclick = () => runNotebook();
addEventListener('keydown', e => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); if (!$('run').disabled) $('run').click(); } });
addCell('x := 6.0\ny := math.Sqrt(x * x + 64)\nfmt.Printf("The result is %.1f\\n", y)');
addCell('for i := 1; i <= 4; i++ {\n\tfmt.Printf("%d squared = %d\\n", i, i*i)\n}');
`
