'use strict';
(() => {
 const panel=$('memory-panel');
 let state=null, source=null, initialNotebook=null, pending=false, stopping=false, foreign=false, cache=new Map(), replayHistory=null, fileStore=new Map(), instance=null, observedHistory=[];
 const key=n=>JSON.stringify({version:n.version,imports:n.imports,cells:n.cells,dependencies:Object.fromEntries(Object.entries(n.dependencies||{}).sort(([a],[b])=>a.localeCompare(b))),files:n.files||[]});
 const stale=()=>{try{return source!==key(notebook())}catch{return true}};
 async function request(action,body) {
  const response=await fetch('/api/native/'+action,{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  const text=await response.text();let value;try{value=JSON.parse(text)}catch{value={error:text}}
  if(!response.ok)throw Error(value.error||text||'Memory kernel request failed');return value;
 }
 function controls() {
  const active=Boolean(state?.active), changed=active&&(foreign||stale());
  $('memory-start').disabled=pending||active||debugging||busy||!token;
  $('memory-stop').disabled=!active&&!pending;$('memory-restart').disabled=pending||busy||debugging||!token;
  $('memory-refresh').disabled=pending||!token;$('memory-restore').hidden=!foreign||!state?.notebook;
  $('memory-replay').hidden=!replayHistory?.length;$('memory-replay').disabled=pending||active||!token;
  $('memory-state').textContent=pending?(stopping?'Stopping kernel…':'Compiling or executing native Go…'):active?(changed?'Source changed · restart required':state.ready?'Paused · native values retained in RAM':'Kernel is running'):'No live memory kernel';
  $('memory-stats').textContent=active?'Go heap at last stop: '+(state.heapAlloc/1048576).toFixed(2)+' MiB · '+state.goroutines+' goroutines · last cell '+state.currentCell:'Stopping or restarting discards live values. Export data and models before doing so.';
  $('memory-identity').textContent=state?.fingerprint?'Source '+state.fingerprint.slice(0,12)+' · '+(state.history?.length||0)+' executed commands':'';
  const cannotRun=busy||pending||debugging||loadingExample||!token||(active&&changed);
  $('run').disabled=cannotRun;document.querySelectorAll('.run-cell').forEach(button=>button.disabled=cannotRun);
  document.querySelectorAll('.run-cell').forEach(button=>{button.textContent=active?'▶ Run cell':'▶ Run to here';button.title=active?'Run this cell in the resident kernel; earlier values stay in RAM':'Start fresh and run all cells up to this one'});
  document.querySelector('.intro-bottom > span').textContent=active?'● Native memory session · values retained':'◷ Fresh process on every run';
  document.querySelectorAll('.cell-foot').forEach(el=>el.textContent=active?'Shift Enter · native values stay in RAM; later cells are invalidated':'Shift Enter · Run to here repeats earlier cells in a fresh process');
  if(active) $('kernel-label').textContent=changed?'Memory kernel · restart required':'Native memory kernel';
  $('debug-start').disabled=active||pending;
 }
 function syncFiles(files,delta=[],cell=0) {
  const changed=new Set(delta.map(a=>a.name));
  if(Array.isArray(files)) {
   validateResult({stdout:'',stderr:'',artifacts:files});
   const next=new Map();for(const artifact of files){const previous=fileStore.get(artifact.name);const owner=changed.has(artifact.name)?cell:previous?.artifact.data===artifact.data?previous.owner:0;next.set(artifact.name,{artifact,owner})}fileStore=next;
  }else for(const artifact of delta){fileStore.delete(artifact.name);fileStore.set(artifact.name,{artifact,owner:cell})}
  let bytes=[...fileStore.values()].reduce((sum,item)=>sum+item.artifact.data.length,0);
  while(fileStore.size>16||bytes>12*1048576){const first=fileStore.keys().next().value;bytes-=fileStore.get(first).artifact.data.length;fileStore.delete(first)}
  $('memory-files').replaceChildren();
  for(const {artifact,owner} of fileStore.values()) {const row=document.createElement('li'),label=document.createElement('span'),button=document.createElement('button');label.textContent=artifact.name+(owner?' · last changed in cell '+owner:' · retained session file');button.textContent='Download';button.onclick=()=>download(Uint8Array.from(atob(artifact.data),c=>c.charCodeAt(0)),artifact.name,artifact.mime);row.append(label,button);$('memory-files').append(row)}
 }
 function update(value,local=false) {
  const changedInstance=instance&&value.sessionId&&instance!==value.sessionId;
  const changedHistory=instance&&value.sessionId===instance&&JSON.stringify(value.history||[])!==JSON.stringify(observedHistory);
  if(!local&&(changedInstance||changedHistory)) {cache.clear();fileStore.clear();clearCellOutputs();source=null;initialNotebook=null;foreign=Boolean(value.active);setExperiment();exportText();emptyOutput('Kernel state changed in another page.','Restore the kernel notebook before continuing. Earlier cell outputs were not recovered.');notify('Kernel state changed in another page. Cached outputs were cleared.');}
  state=value;instance=value.sessionId||null;observedHistory=[...(value.history||[])];if(!local)syncFiles(value.files);controls();
 }
 function open() {panel.hidden=false;$('memory-toggle').setAttribute('aria-expanded','true');setView('both');panel.scrollIntoView({block:'nearest'});controls()}
 function relevantCurrent(){const n=notebook();delete n.runThrough;return n}
 function mergedResult(last) {
  let textBudget=1048576,remainingCells=cache.size;const cells=[];
  for(const [number,item] of [...cache].sort(([a],[b])=>b-a)) {
   const available=Math.max(0,textBudget-(--remainingCells)*100),c={cell:number,stdout:item.stdout,stderr:item.stderr,artifacts:[]};
   if(c.stdout.length+c.stderr.length>available){c.stdout='Earlier output omitted from the display budget. Run this cell again to refresh it.';c.stderr=''}
   textBudget=Math.max(0,textBudget-c.stdout.length-c.stderr.length);
   for(const item of fileStore.values())if(item.owner===number)c.artifacts.push(item.artifact);
   cache.set(number,c);cells.push(c);
  }
  cells.sort((a,b)=>a.cell-b.cell);
  return {...last,stdout:cells.map(c=>c.stdout).join(''),stderr:cells.map(c=>c.stderr).join(''),cells,artifacts:[...fileStore.values()].map(item=>item.artifact)};
 }
 function showResult(result,cell) {
  if(!result)return;
  for(const number of cache.keys())if(number>cell)cache.delete(number);
  syncFiles(state.files,result.artifacts||[],cell);
  for(const item of result.cells||[])cache.set(item.cell,structuredClone(item));
  const combined=mergedResult(result),matches=!foreign&&!stale();renderResult(combined,matches);
  if(initialNotebook) setExperiment({format:'gobook-experiment',version:1,notebook:initialNotebook,result:combined,session:{kind:'native',id:state.sessionId,fingerprint:state.fingerprint,history:[...(state.history||[])]}});
  $('execution-note').textContent='Native memory session · outputs reflect recorded execution order. Retained files may predate the latest cell. Saved bundles do not contain live RAM.';
 }
 async function start() {
  if(pending||state?.active||debugging||busy||!token)return false;
  let n;try{n=relevantCurrent()}catch(error){notify(error.message);return false}
  pending=true;stopping=false;setBusy(true);controls();$('memory-error').textContent='';
  try{
   const value=await request('start',{notebook:n});update(value,true);
   if(value.error||!value.active){if(value.result)renderResult(value.result);throw Error(value.error||'Kernel did not start')}
   source=key(n);initialNotebook=structuredClone(n);foreign=false;cache.clear();fileStore.clear();syncFiles(value.files);clearCellOutputs();if(value.result&&(value.result.stdout||value.result.stderr||value.result.artifacts?.length))renderResult(value.result,false);
   setStatus('Memory kernel ready · nothing executed');return true;
  }catch(error){$('memory-error').textContent=error.message;return false}
  finally{pending=false;setBusy(false);controls()}
 }
 async function stop() {
  if(stopping||(!state?.active&&!pending))return false;
  stopping=true;controls();
  try {update(await request('stop',{sessionId:state?.active?state.sessionId:undefined}),true);setStatus('Memory kernel stopped · live values discarded');return true}
  catch(error){$('memory-error').textContent=error.message;return false}
  finally{stopping=false;controls()}
 }
 async function run(through=null) {
  if(pending||busy||debugging||loadingExample||!state?.active)return false;
  if(foreign||stale()){open();notify('Restore the kernel notebook or restart after your source changes.');return}
  const target=through===null?initialNotebook.cells.length:through;
  let queue=through===null?Array.from({length:target},(_,i)=>i+1):target>state.nextCell?Array.from({length:target-state.nextCell+1},(_,i)=>state.nextCell+i):[target];
  pending=true;stopping=false;setBusy(true);controls();$('memory-error').textContent='';let success=false;
  try {
   for(const cell of queue) {
    if(stopping||!state.active)break;
    if(stale())throw Error('Source changed during execution. Restart is required before the next cell.');
    setStatus('Memory kernel · executing cell '+cell);
    const previousCount=state.history?.length||0,previousID=state.sessionId;
    const value=await request('execute',{notebook:initialNotebook,cell,sessionId:previousID,historyLength:previousCount});
    const accepted=value.commandAccepted===true&&value.sessionId===previousID&&(value.history?.length||0)===previousCount+1;update(value,accepted);
    if(accepted)showResult(value.result,cell);else if(!value.active&&value.result&&source&&!foreign)showResult(value.result,value.currentCell);
    if(value.error||!value.active)throw Error(value.error||'The memory kernel ended');
   }
   success=Boolean(state?.active&&!stopping);
   setStatus(state?.active?'Memory kernel paused · cell '+state.currentCell:'Memory kernel stopped',state?.active?'success':'');
  }catch(error){$('memory-error').textContent=error.message;setStatus('Memory execution stopped','error')}
  finally{pending=false;setBusy(false);controls();if(through!==null&&!stale())$('cells').children[through-1]?.querySelector('.cell-output')?.scrollIntoView({block:'nearest'})}
  return success;
 }
 async function recover() {
  if(!token||pending)return;
  try {const value=await request('state');if(value.active&&source===null){foreign=true;open()}update(value);if(value.error){$('memory-error').textContent=value.error;if(source&&!foreign&&value.result)showResult(value.result,value.currentCell)}}catch(error){$('memory-error').textContent=error.message}
 }
 $('memory-toggle').onclick=()=>{panel.hidden=!panel.hidden;$('memory-toggle').setAttribute('aria-expanded',String(!panel.hidden));if(!panel.hidden){setView('both');recover();controls()}};
 $('memory-start').onclick=start;$('memory-stop').onclick=stop;$('memory-refresh').onclick=recover;
 $('memory-restart').onclick=async()=>{if(pending)return;if(state?.active&&!confirm('Restart the memory kernel? Live values, handles and goroutines will be discarded. Export data or models first.'))return;if(state?.active&&!await stop())return;await start()};
 $('memory-restore').onclick=()=>{if(!state?.notebook)return;if(changed&&!confirm('Replace this page with the kernel source? Save your current edits first.'))return;initialNotebook=structuredClone(state.notebook);source=key(initialNotebook);foreign=false;cache.clear();fileStore.clear();syncFiles(state.files);load(initialNotebook);controls();notify('Kernel source restored. Live values remain in the resident process.')};
 $('memory-replay').onclick=async()=>{const history=[...replayHistory];if(!await start())return;for(const cell of history){if(!state?.active||stopping)break;if(!await run(cell))break}};
 $('applytoken').addEventListener('click',recover);
 globalThis.GoBookMemory={get active(){return Boolean(state?.active)},run,sourceChanged:()=>controls(),importedSession:session=>{replayHistory=session?.kind==='native'&&Array.isArray(session.history)&&session.history.length<=1000&&session.history.every(i=>Number.isInteger(i)&&i>0&&i<=$('cells').children.length)?[...session.history]:null;controls()}};
 recover();
})();
