const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
function controller(ok=true) {
 const elements=new Map(),calls=[];
 const $=id=>{if(!elements.has(id))elements.set(id,{hidden:true,disabled:false,textContent:'',onclick:null,addEventListener(){}});return elements.get(id)};
 const c={$,token:'private',document:{body:{dataset:{}}},sessionStorage:{removeItem:key=>calls.push(key)},updateConnection:()=>calls.push('update'),setStatus:()=>{},notify:()=>calls.push('notify'),confirm:()=>true,
 fetch:async(path,options)=>{calls.push({path,options});return {ok,json:async()=>({app:'gobook',desktop:true})}}};
 vm.createContext(c);
 vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../desktop_ui.js'),'utf8').replace('\n detect();','\n globalThis.detect=detect;'),c);
 return {c,$,calls};
}
test('local host detection removes manual connection and quitting clears execution capability',async()=>{
 const h=controller();await h.c.detect();assert.equal(h.$('local-app').hidden,false);assert.equal(h.c.document.body.dataset.localApp,'true');
 await h.$('local-app-quit').onclick();assert.equal(h.c.token,'');assert.ok(h.calls.includes('gobook-token'));assert.match(h.$('local-app-state').textContent,/Stopped/);
 assert.equal(h.calls.filter(c=>c.path==='/api/desktop/quit').length,1);
});
test('ordinary host keeps manual flow and cancel never stops a host',async()=>{
 const ordinary=controller(false);await ordinary.c.detect();assert.equal(ordinary.$('local-app').hidden,true);
 const h=controller();await h.c.detect();h.c.confirm=()=>false;await h.$('local-app-quit').onclick();assert.equal(h.c.token,'private');assert.equal(h.calls.filter(c=>c.path==='/api/desktop/quit').length,0);
});
