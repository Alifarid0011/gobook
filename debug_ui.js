'use strict';
(() => {
 let active = false, pending = false, foreignSession = false, snapshot = null, pollTimer = null;
 const panel = $('debug-panel'); $('workspace').append(panel);
 let lastStop = ''; const previousValues = new Map();
 function openPanel() { panel.hidden=false; document.body.classList.add('debug-open'); setView('both'); }
 function jump(cellNumber,line) { const cell=$('cells').children[cellNumber-1]; if (!cell) return; const editor=cell.querySelector('textarea'); editorViews.get(editor)?.markLine(line); cell.scrollIntoView({block:'center'}); editor.focus({preventScroll:true}); }
 $('cells').addEventListener('gobook-breakpoint',event => { if (active || pending) return; const cell=event.target.closest('.cell'); $('break-cell').value=[...$('cells').children].indexOf(cell)+1; $('break-line').value=event.detail.line; openPanel(); $('debug-status').textContent='Breakpoint selected · cell '+$('break-cell').value+', line '+event.detail.line+'. Start debugging when ready.'; });
 async function request(action, body) {
  const response = await fetch('/api/debug/'+action, {method:body === undefined ? 'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body === undefined ? undefined:JSON.stringify(body)});
  const text = await response.text(); let value; try { value = JSON.parse(text); } catch { value = {error:text}; }
  if (!response.ok) throw Error(value.error || text || 'Debugger request failed'); return value;
 }
 function lock(value) { debugging = value; setBusy(busy); $('debug-close').disabled=pending; }
 function highlight(state) {
  document.querySelectorAll('.cell textarea').forEach((editor,i) => editorViews.get(editor)?.markLine(!foreignSession && !state.running && i+1 === state.cell ? state.line : 0));
 }
 function variables(parent, list, depth = 0) {
  parent.replaceChildren();
  for (const item of (list || []).slice(0,50)) {
   const row = document.createElement('div'); row.className = 'debug-variable'; const name = document.createElement('strong'); name.textContent = item.name;
   if (depth === 0) { const key=item.name+':'+item.type, value=JSON.stringify([item.value,item.children,item.unreadable]), before=previousValues.get(key); const until=before && before.value!==value ? Date.now()+1800 : before?.until || 0; previousValues.set(key,{value,until}); row.classList.toggle('value-changed',until>Date.now()); }
   const type = document.createElement('small'); type.textContent = item.type; const value = document.createElement('code'); value.textContent = item.unreadable || item.value || (item.children?.length ? '(expand below)' : ''); row.append(name,type,value);
   if (depth < 2 && item.children?.length) { const details = document.createElement('details'); const summary = document.createElement('summary'); summary.textContent = 'Fields / elements'; const children = document.createElement('div'); variables(children,item.children,depth+1); details.append(summary,children); row.append(details); }
   parent.append(row);
  }
  if (!parent.childNodes.length) { const empty = document.createElement('p'); empty.textContent = 'No local variables available at this stop.'; parent.append(empty); }
 }
 function render(state) {
  snapshot = state; active = !state.exited; panel.dataset.state = state.exited ? 'ended' : state.running ? 'running' : 'paused';
  if (!active) foreignSession = false;
  $('debug-status').textContent = state.error || (state.exited ? 'Debug session ended.' : state.running ? 'Running · live status updates' : state.cell ? 'Paused · cell '+state.cell+', line '+state.line+' · '+state.function : 'Paused · '+(state.function || 'program entry'));
  if (foreignSession) $('debug-status').textContent = 'An existing debug session belongs to another page snapshot. Stop it before debugging this notebook. Its source has not been restored here.';
  $('debug-output').textContent = [state.stdout,state.stderr,state.truncated ? 'Output truncated at 1 MiB.':''].filter(Boolean).join('\n');
  variables($('debug-locals'),state.running ? [] : [...(state.arguments || []),...(state.locals || [])]);
  $('debug-stack').replaceChildren();
  if (!state.running) for (const frame of state.stack || []) { const li = document.createElement('li'); li.textContent = frame.function+' · '+(frame.cell ? 'cell '+frame.cell : frame.file?.split(/[\\/]/).pop())+':'+frame.line; $('debug-stack').append(li); }
  document.querySelectorAll('[data-debug]').forEach(button => { button.disabled = foreignSession || pending || !active || (button.dataset.debug === 'halt' ? !state.running : state.running); });
  $('debug-stop').disabled = pending || !active; $('debug-launch').disabled = pending || active || busy || !token; $('break-cell').disabled = active || pending; $('break-line').disabled = active || pending;
  $('debug-jump').disabled = foreignSession || !active || !state.cell || state.running; highlight(state); lock(active);
  const stopKey = state.cell+':'+state.line;
  if (active && !state.running && !foreignSession && state.cell && lastStop !== stopKey) { lastStop=stopKey; jump(state.cell,state.line); }
  if (state.running) lastStop='';
 }
 function schedule() {
  clearTimeout(pollTimer); if (!active || foreignSession) return;
  pollTimer = setTimeout(async () => { try { if (!pending) render(await request('state')); } catch (e) { $('debug-status').textContent = e.message+' · use Stop to release the session.'; } schedule(); },500);
 }
 $('debug-start').onclick = openPanel;
 $('debug-launch').onclick = async () => {
  if (pending || active || busy || !token) return;
  let n; try { n = notebook(); } catch (e) { $('debug-status').textContent=e.message; return; }
  const cell = Number($('break-cell').value), line = Number($('break-line').value);
  if (!Number.isInteger(cell) || cell < 1 || cell > n.cells.length || !Number.isInteger(line) || line < 1) { $('debug-status').textContent='Choose a valid cell and line.'; return; }
  pending = true; clearCellOutputs(); lastStop=''; previousValues.clear(); panel.dataset.state='building'; lock(true); $('debug-launch').disabled = true; $('debug-status').textContent = 'Building without optimizations and starting Delve…';
  try { const state = await request('start',{notebook:n,breakpoints:[{cell,line}]}); pending=false; render(state); schedule(); }
  catch (e) { pending=false; active=false; lock(false); $('debug-status').textContent=e.message; $('debug-launch').disabled=false; }
 };
 document.querySelectorAll('[data-debug]').forEach(button => button.onclick = async () => {
  if (pending || !active) return; pending=true; document.querySelectorAll('[data-debug]').forEach(b=>b.disabled=true);
  try { const state=await request('command',{command:button.dataset.debug}); pending=false; render(state); schedule(); }
  catch (e) { pending=false; $('debug-status').textContent=e.message; schedule(); }
 });
 async function stop() {
  if (pending) return; pending=true; clearTimeout(pollTimer);
  try { const state=await request('stop',{}); pending=false; active=false; render(state); }
  catch (e) { pending=false; $('debug-status').textContent=e.message; }
 }
 $('debug-stop').onclick = stop;
 $('debug-close').onclick = async () => { if (pending) return; if (active) await stop(); if (!active) { panel.hidden=true; document.body.classList.remove('debug-open'); } };
 $('debug-jump').onclick = () => {
  if (!snapshot?.cell) return; jump(snapshot.cell,snapshot.line);
 };
 async function recover() {
  if (!token || busy || pending || active || loadingExample) return;
  try { const state = await request('state'); if (!state.exited) { foreignSession=true; openPanel(); render(state); } }
  catch { /* Optional debugger availability must not prevent ordinary notebooks. */ }
 }
 addEventListener('keydown',event => {
  if (!active || pending || event.altKey || event.ctrlKey || event.metaKey) return;
  const command = event.key === 'F8' ? (snapshot?.running ? 'halt':'continue') : event.key === 'F10' ? 'next' : event.key === 'F11' ? (event.shiftKey ? 'stepOut':'step') : null;
  if (!command) return; event.preventDefault(); const button=document.querySelector('[data-debug="'+command+'"]'); if (!button.disabled) button.click();
 });
 $('applytoken').addEventListener('click',recover);
 recover();
})();
