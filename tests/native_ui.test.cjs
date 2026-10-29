const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

// Exercise the shipped controller without a browser or a duplicated controller.
function controller() {
 const elements=new Map();
 const element=()=>({disabled:false,hidden:false,textContent:'',children:[],setAttribute(){},addEventListener(){},scrollIntoView(){},replaceChildren(){this.children=[]},append(...nodes){this.children.push(...nodes)}});
 const $=id=>{if(!elements.has(id))elements.set(id,element());return elements.get(id)};
 const runCell=element();let current={version:1,imports:['fmt'],cells:[{code:'fmt.Println(1)'}]},renders=[],cleared=0;
 const context={console,Map,Set,JSON,Object,Array,Boolean,Number,Math,Error,structuredClone,Uint8Array,atob,$,token:'test',debugging:false,busy:false,loadingExample:false,changed:false,
  notebook:()=>structuredClone(current),document:{querySelectorAll:s=>s==='.run-cell'?[runCell]:[],querySelector:()=>element(),createElement:element},
  validateResult:r=>assert.ok(Array.isArray(r.artifacts)),clearCellOutputs:()=>cleared++,setExperiment(){},exportText(){},emptyOutput(){},notify(){},renderResult:r=>renders.push(r),download(){},setView(){},setBusy(){},setStatus(){},load:n=>{current=n}};
 vm.createContext(context);
 const code=fs.readFileSync(require('node:path').join(__dirname,'../native_ui.js'),'utf8').replace('\n recover();',`\n globalThis.check={run,controls,update,showResult,syncFiles,mergedResult,set(n,s){source=key(n);initialNotebook=structuredClone(n);state=s;instance=s.sessionId;observedHistory=[...(s.history||[])];foreign=false},cache,files:()=>fileStore,foreign:()=>foreign};`);
 vm.runInContext(code,context);
 return {context,check:context.check,$,runCell,current:n=>{current=n},renders,cleared:()=>cleared,notebook:()=>structuredClone(current)};
}
const file={name:'model.json',mime:'application/json',data:'e30='};
const snapshot=(history=[])=>({sessionId:'one',active:true,ready:true,history,heapAlloc:0,goroutines:2,currentCell:history.at(-1)||0,files:[]});
const result=(cell,artifacts=[])=>({stdout:'ok',stderr:'',cells:[{cell,stdout:'ok',stderr:'',artifacts}],artifacts});

test('Undo and stop restore execution controls',()=>{
 const h=controller(),n=h.notebook();h.check.set(n,snapshot());h.check.controls();assert.equal(h.$('run').disabled,false);
 h.current({...n,cells:[{code:'changed'}]});h.check.controls();assert.equal(h.runCell.disabled,true);
 h.current(n);h.check.controls();assert.equal(h.runCell.disabled,false);
 h.current({...n,cells:[{code:'changed'}]});h.check.controls();h.check.update({...snapshot(),active:false},true);assert.equal(h.$('run').disabled,false);
});
test('unchanged files survive reruns and invalidation; complete empty manifest removes files',()=>{
 const h=controller();h.check.set(h.notebook(),{...snapshot([1]),files:[file]});h.check.showResult(result(1,[file]),1);
 h.check.update({...snapshot([1,1]),files:[file]},true);h.check.showResult(result(1),1);
 assert.equal(h.renders.at(-1).cells[0].artifacts.length,1);
 h.check.update({...snapshot([1,1,2]),files:[file]},true);h.check.showResult(result(2),2);
 h.check.update({...snapshot([1,1,2,1]),files:[file]},true);h.check.showResult(result(1),1);
 assert.equal(h.renders.at(-1).artifacts.length,1);assert.equal(h.renders.at(-1).cells.length,1);
 h.check.update({...snapshot([1,1,2,1,1]),files:[]},true);h.check.showResult(result(1),1);assert.equal(h.renders.at(-1).artifacts.length,0);
});
test('partial file scan preserves known files and replacement history clears old outputs',()=>{
 const h=controller();h.check.set(h.notebook(),{...snapshot([1]),files:[file]});h.check.showResult(result(1,[file]),1);
 h.check.syncFiles(null);assert.equal(h.check.files().size,1);
 h.check.update({...snapshot([1,2]),files:[file]});assert.equal(h.check.cache.size,0);assert.equal(h.check.foreign(),true);assert.equal(h.$('run').disabled,true);assert.ok(h.cleared()>0);
 h.check.update({...snapshot(),sessionId:'two',files:[]});assert.equal(h.check.files().size,0);
});

test('rejected command with one remote history entry never mixes remote and cached outputs',async()=>{
 const h=controller();h.check.set(h.notebook(),snapshot([1]));h.check.showResult(result(1),1);
 h.context.fetch=async()=>({ok:true,text:async()=>JSON.stringify({...snapshot([1,1]),error:'native session changed in another page',result:result(1)})});
 await h.check.run(1);assert.equal(h.check.foreign(),true);assert.equal(h.check.cache.size,0);assert.equal(h.renders.length,1);
});

test('stopped local app keeps debugger disabled without a capability',()=>{
 const h=controller();h.context.token='';h.check.controls();assert.equal(h.$('debug-start').disabled,true);assert.equal(h.$('run').disabled,true);
});

test('selected cell never replays a missing prefix',async()=>{
 const h=controller(),n={...h.notebook(),cells:[{code:'x:=1'},{code:'x++'},{code:'fmt.Println(x)'}]};
 h.current(n);h.check.set(n,{...snapshot(),nextCell:1});const sent=[];
 h.context.fetch=async(path,options)=>{sent.push(JSON.parse(options.body));return {ok:true,text:async()=>JSON.stringify({...snapshot(),nextCell:1,error:'cell 3 unavailable'})}};
 await h.check.run(3);assert.deepEqual(sent.map(r=>r.cell),[3]);
});

test('selected cell starts memory kernel automatically without executing other cells',async()=>{
 const h=controller(),sent=[];
 h.context.fetch=async(path,options)=>{sent.push({path,body:JSON.parse(options.body)});return {ok:true,text:async()=>JSON.stringify(path.endsWith('/start')?{...snapshot(),nextCell:1}:{...snapshot([1]),nextCell:1,commandAccepted:true,result:result(1)})}};
 assert.equal(await h.check.run(1),true);assert.deepEqual(sent.map(r=>r.path),['/api/native/start','/api/native/execute']);assert.equal(sent[1].body.cell,1);
});

test('interactive cell edits send current source without resetting session or rerunning other cells',async()=>{
 const h=controller(),n=h.notebook();h.check.set(n,{...snapshot([1]),engine:'interactive'});
 h.current({...n,cells:[{code:'fmt.Println(2)'}]});h.check.controls();assert.equal(h.runCell.disabled,false);
 let sent;h.context.fetch=async(path,options)=>{sent=JSON.parse(options.body);return {ok:true,text:async()=>JSON.stringify({...snapshot([1,1]),engine:'interactive',commandAccepted:true,result:result(1)})}};
 await h.check.run(1);assert.equal(sent.notebook.cells[0].code,'fmt.Println(2)');assert.equal(sent.sessionId,'one');assert.equal(sent.historyLength,1);
});
test('interactive rerun preserves later cell output instead of applying native frontier invalidation',()=>{
 const h=controller();h.check.set(h.notebook(),{...snapshot([1,2]),engine:'interactive'});h.check.showResult(result(2),2);h.check.showResult(result(1),1);
 assert.deepEqual(Array.from(h.check.cache.keys()).sort(),[1,2]);
});

test('edited-during-run interactive output is historical, never inline under changed code',async()=>{
 const h=controller(),n=h.notebook();h.check.set(n,{...snapshot([1]),engine:'interactive'});
 h.context.fetch=async()=>{h.current({...n,cells:[{code:'new source'}]});return {ok:true,text:async()=>JSON.stringify({...snapshot([1,1]),engine:'interactive',commandAccepted:true,result:result(1)})}};
 await h.check.run(1);assert.equal(h.renders.at(-1).cells.length,0);
});
test('saved native replay selects native and edited interactive history is not offered for replay',()=>{
 const h=controller();h.context.GoBookMemory.importedSession({kind:'native',history:[1]});assert.equal(h.$('memory-engine').value,'native');
 h.context.GoBookMemory.importedSession({kind:'interactive',history:[1,1]});assert.equal(h.$('memory-replay').hidden,true);assert.equal(h.$('memory-engine').value,'interactive');
});
test('invalid in-progress configuration text cannot break live editor change handling',()=>{
 const h=controller();h.check.set(h.notebook(),{...snapshot(),engine:'interactive'});h.context.notebook=()=>{throw Error('incomplete config')};assert.doesNotThrow(()=>h.context.GoBookMemory.sourceChanged());
});

test('module examples select native only before a kernel starts',()=>{
 const h=controller(),n=h.notebook();h.context.GoBookMemory.loadedNotebook({...n,imports:['github.com/Alifarid0011/sheetframe']});assert.equal(h.$('memory-engine').value,'native');
 h.context.GoBookMemory.loadedNotebook(n);assert.equal(h.$('memory-engine').value,'interactive');
 h.check.set(n,{...snapshot(),engine:'interactive'});h.context.GoBookMemory.loadedNotebook({...n,imports:['github.com/Alifarid0011/sheetframe']});assert.equal(h.$('memory-engine').value,'interactive');
});
