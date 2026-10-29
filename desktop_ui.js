'use strict';
(() => {
 let local=false;
 async function detect() {
  if(!token)return;
  try {
   const response=await fetch('/api/desktop/status',{headers:{Authorization:'Bearer '+token}});
   if(!response.ok)return;
   const state=await response.json();if(state.app!=='gobook'||!state.desktop)return;
   local=true;document.body.dataset.localApp='true';$('local-app').hidden=false;$('connection').hidden=true;
   $('local-app-state').textContent='Connected automatically · runs on this computer';
  }catch { /* Ordinary CLI and remote hosts do not offer desktop lifecycle. */ }
 }
 $('local-app-quit').onclick=async()=>{
  if(!local||!confirm('Quit GoBook Local? Save your notebook first. Live kernel values will be discarded.'))return;
  $('local-app-quit').disabled=true;
  try {
   const response=await fetch('/api/desktop/quit',{method:'POST',headers:{Authorization:'Bearer '+token}});
   if(!response.ok)throw Error('Could not stop GoBook.');
   local=false;token='';sessionStorage.removeItem('gobook-token');updateConnection();globalThis.GoBookMemory?.sourceChanged();
   $('local-app-state').textContent='Stopped · open GoBook Local from your application menu to restart';
   setStatus('Local app stopped');
  }catch(error){$('local-app-quit').disabled=false;notify(error.message)}
 };
 $('applytoken').addEventListener('click',detect);
 detect();
})();
