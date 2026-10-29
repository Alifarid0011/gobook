package main

const appJS = `
'use strict';
const $ = id => document.getElementById(id);
let token = new URL(location.href).searchParams.get('token') || sessionStorage.getItem('gobook-token') || '';
sessionStorage.setItem('gobook-token', token);
history.replaceState(null, '', '/');
let busy = false, changed = false, toastTimer, runTimer;
const mac = /Mac|iPhone|iPad/.test(navigator.platform);
$('keyboard-shortcut').textContent = (mac ? '⌘' : 'Ctrl') + ' Enter';
document.querySelector('.shortcut').textContent = mac ? '⌘ ↵' : 'Ctrl ↵';
function notify(message) {
 $('toast').textContent = message; $('toast').hidden = false;
 clearTimeout(toastTimer); toastTimer = setTimeout(() => $('toast').hidden = true, 3500);
}
function setStatus(message, state = '') { $('status').textContent = message; $('status').dataset.state = state; }
function updateConnection() {
 $('run').disabled = busy || !token;
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
['title', 'note', 'imports'].forEach(id => $(id).addEventListener('input', markChanged));
addEventListener('beforeunload', e => { if (changed) { e.preventDefault(); e.returnValue = ''; } });
function addCell(code = '', focus = false) {
 const section = document.createElement('article'); section.className = 'cell';
 const head = document.createElement('div'); head.className = 'cellhead';
 const label = document.createElement('span'); label.className = 'cell-label';
 const actions = document.createElement('div'); actions.className = 'cell-actions';
 const up = document.createElement('button'), down = document.createElement('button'), remove = document.createElement('button');
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
 actions.append(up, down, remove); head.append(label, actions);
 const wrap = document.createElement('div'); wrap.className = 'editor-wrap';
 const lines = document.createElement('div'); lines.className = 'line-numbers'; lines.setAttribute('aria-hidden', 'true');
 const editor = document.createElement('textarea'); editor.value = code; editor.spellcheck = false; editor.autocapitalize = 'off'; editor.autocomplete = 'off'; editor.wrap = 'off';
 function updateLines() { lines.textContent = Array.from({ length: editor.value.split('\n').length }, (_, i) => i + 1).join('\n'); lines.scrollTop = editor.scrollTop; }
 editor.addEventListener('input', () => { updateLines(); markChanged(); });
 editor.addEventListener('scroll', () => { lines.scrollTop = editor.scrollTop; });
 editor.addEventListener('keydown', e => {
  if (e.key === 'Tab' && !e.shiftKey) { e.preventDefault(); editor.setRangeText('\t', editor.selectionStart, editor.selectionEnd, 'end'); updateLines(); markChanged(); }
 });
 const foot = document.createElement('div'); foot.className = 'cell-foot'; foot.textContent = 'Go · executes with all cells';
 wrap.append(lines, editor); section.append(head, wrap, foot); $('cells').append(section); updateLines(); renumber();
 editor.style.height = Math.min(420, Math.max(128, (code.split('\n').length + 2) * 20)) + 'px';
 if (focus) { editor.focus(); section.scrollIntoView({ block: 'nearest' }); }
}
function renumber() {
 const cells = [...$('cells').children]; $('outline').replaceChildren();
 cells.forEach((cell, i) => {
  cell.id = 'cell-' + (i + 1); cell.querySelector('.cell-label').textContent = 'CELL ' + String(i + 1).padStart(2, '0');
  cell.querySelector('textarea').setAttribute('aria-label', 'Go code cell ' + (i + 1));
  const buttons = cell.querySelectorAll('.cell-actions button'); buttons[0].disabled = i === 0; buttons[1].disabled = i === cells.length - 1;
  const item = document.createElement('button'); const number = document.createElement('em'); number.textContent = String(i + 1).padStart(2, '0');
  item.append(number, document.createTextNode('Code cell ' + (i + 1))); item.onclick = () => { cell.scrollIntoView({ block: 'center' }); cell.querySelector('textarea').focus({ preventScroll: true }); };
  $('outline').append(item);
 });
 $('cell-count').textContent = cells.length + ' code cell' + (cells.length === 1 ? '' : 's');
}
function notebook() { return { version: 1, title: $('title').value, note: $('note').value, imports: $('imports').value.split(',').map(s => s.trim()).filter(Boolean), cells: [...document.querySelectorAll('.cell textarea')].map(t => ({ code: t.value })) }; }
function emptyOutput(title = 'Make room for a discovery.', note = 'Run your notebook to see its output and figures here.') {
 const empty = document.createElement('div'); empty.className = 'empty';
 const icon = document.createElement('span'); icon.className = 'empty-symbol'; icon.textContent = '↗';
 const strong = document.createElement('strong'); strong.textContent = title;
 const p = document.createElement('p'); p.textContent = note; empty.append(icon, strong, p); $('output').replaceChildren(empty);
}
function load(n) {
 if (!n || n.version !== 1 || typeof n.title !== 'string' || typeof n.note !== 'string' || !Array.isArray(n.imports) || !n.imports.every(x => typeof x === 'string') || !Array.isArray(n.cells) || !n.cells.every(c => c && typeof c.code === 'string')) throw Error('Invalid version 1 notebook');
 $('title').value = n.title; $('note').value = n.note; $('imports').value = n.imports.join(', '); $('import-summary').textContent = $('imports').value || 'No imports';
 $('cells').replaceChildren(); n.cells.forEach(c => addCell(c.code)); renumber();
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
$('save').onclick = () => {
 const blob = new Blob([JSON.stringify(notebook(), null, 2)], { type: 'application/json' }); const url = URL.createObjectURL(blob); const a = document.createElement('a');
 a.href = url; a.download = ($('title').value.trim().replace(/[^a-zA-Z0-9_-]+/g, '-').slice(0, 80) || 'notebook') + '.gobook.json'; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
 changed = false; $('save-state').textContent = 'Downloaded as JSON'; notify('Notebook downloaded as JSON.');
};
$('load').onclick = () => $('file').click();
$('file').onchange = async () => {
 const file = $('file').files[0]; if (!file) return;
 try { if (file.size > 262144) throw Error('Notebook exceeds 256 KiB'); if (changed && !confirm('Open this notebook? Save your current changes first.')) return; load(JSON.parse(await file.text())); notify('Notebook opened. Code has not been run.'); }
 catch (e) { notify(e.message); } finally { $('file').value = ''; }
};
function textOutput(label, text, error = false) {
 if (!text) return; const title = document.createElement('p'); title.className = 'output-label'; title.textContent = label;
 const pre = document.createElement('pre'); pre.textContent = text; if (error) pre.className = 'error'; $('output').append(title, pre);
}
function setBusy(value) {
 busy = value; document.body.classList.toggle('running', value); $('run-label').textContent = value ? 'Running' : 'Run all';
 for (const id of ['new', 'load', 'add', 'sidebar-new']) $(id).disabled = value;
 document.querySelectorAll('.cell-actions button').forEach(b => b.disabled = value);
 if (!value) renumber(); updateConnection(); $('output').setAttribute('aria-busy', String(value));
}
$('run').onclick = async () => {
 if (busy || !token) return; const snapshot = JSON.stringify(notebook()); const start = performance.now(); setBusy(true);
 emptyOutput('A thought in motion.', 'Compiling and running your Go notebook…');
 const elapsed = () => ((performance.now() - start) / 1000).toFixed(1);
 setStatus('Running · 0.0s'); runTimer = setInterval(() => setStatus('Running · ' + elapsed() + 's'), 1000);
 $('execution-note').textContent = 'Compiling, then executing in a fresh process.';
 try {
  const response = await fetch('/api/run', { method: 'POST', headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' }, body: snapshot });
  if (!response.ok) throw Error(await response.text());
  const result = await response.json(); $('output').replaceChildren();
  textOutput('STANDARD OUTPUT', result.stdout); textOutput('BUILD & RUN DIAGNOSTICS', result.stderr); textOutput('EXECUTION ERROR', result.error, true);
  if (result.truncated) textOutput('OUTPUT LIMIT', 'Output was truncated at 1 MiB.');
  for (const artifact of result.artifacts || []) {
   const figure = document.createElement('figure'); figure.className = 'artifact'; const caption = document.createElement('figcaption'); caption.textContent = artifact.name;
   const img = document.createElement('img'); img.alt = artifact.name; img.src = 'data:' + artifact.mime + ';base64,' + artifact.data; figure.append(caption, img); $('output').append(figure);
  }
  if (!$('output').childNodes.length) emptyOutput('Quietly complete.', 'Your code finished successfully without producing output.');
  clearInterval(runTimer); setStatus((result.error ? 'Failed' : 'Complete') + ' · ' + elapsed() + 's', result.error ? 'error' : 'success');
  $('execution-note').textContent = JSON.stringify(notebook()) !== snapshot ? 'Notebook edited during run · results reflect earlier code.' : (result.error ? 'Review the diagnostics, adjust, and try again.' : 'Fresh process finished. Keep exploring.');
 } catch (e) {
  clearInterval(runTimer); $('output').replaceChildren(); textOutput('REQUEST ERROR', e.message, true); setStatus('Could not run · ' + elapsed() + 's', 'error');
  $('execution-note').textContent = 'Check that your local GoBook host is running.';
 } finally { clearInterval(runTimer); setBusy(false); }
};
addEventListener('keydown', e => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); if (!$('run').disabled) $('run').click(); } });
addCell('x := 6.0\ny := math.Sqrt(x * x + 64)\nfmt.Printf("The result is %.1f\\n", y)');
addCell('for i := 1; i <= 4; i++ {\n\tfmt.Printf("%d squared = %d\\n", i, i*i)\n}');
`
