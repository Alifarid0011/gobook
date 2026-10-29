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
