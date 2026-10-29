const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');
const ctx={};vm.createContext(ctx);vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../completion_ui.js'),'utf8')+'\nglobalThis.contextAt=GoBookCompletion.contextAt;',ctx);
test('completion replacement respects selectors, Unicode, and cursor bounds',()=>{
 assert.deepEqual(JSON.parse(JSON.stringify(ctx.contextAt('fmt.Pr',6))),{prefix:'Pr',start:4,end:6});
 assert.deepEqual(JSON.parse(JSON.stringify(ctx.contextAt('نام.مق',6))),{prefix:'مق',start:4,end:6});
 assert.equal(ctx.contextAt('x',-1),null);assert.equal(ctx.contextAt('x',2),null);
});

// Exercise the shipped attach() handlers with a deferred network response. The
// small DOM surface models only APIs used by the completion menu; timers stay
// under test control so a queued reopen cannot race the assertions.
function harness(text='fmt.Pr',cursor=text.length){
 const timers=new Map(),requests=[];let timerID=0;
 const document={activeElement:null,createElement:()=>new Element()};
 class Element {
  constructor(){this.children=[];this.style={};this.attributes={};this.listeners={};this.isConnected=true;this.readOnly=false;this.clientWidth=500;this.clientHeight=200;this.scrollLeft=0;this.scrollTop=0}
  append(...children){this.children.push(...children)}
  replaceChildren(...children){this.children=[...children]}
  setAttribute(name,value){this.attributes[name]=value}
  removeAttribute(name){delete this.attributes[name]}
  addEventListener(type,handler){(this.listeners[type]??=[]).push(handler)}
  dispatchEvent(event){for(const handler of this.listeners[event.type]||[])handler(event);return true}
  scrollIntoView(){}
  getContext(){return {measureText:text=>({width:text.length*8})}}
  focus(){document.activeElement=this}
  setRangeText(replacement,start,end){this.value=this.value.slice(0,start)+replacement+this.value.slice(end);this.selectionStart=this.selectionEnd=start+replacement.length}
 }
 const sandbox={document,Event:class {constructor(type){this.type=type}},
  getComputedStyle:()=>({lineHeight:'20',paddingLeft:'5',paddingTop:'5',font:'14px monospace',tabSize:'4'}),
  setTimeout:fn=>{const id=++timerID;timers.set(id,fn);return id},clearTimeout:id=>timers.delete(id),
  fetch:(_url,options)=>new Promise(resolve=>requests.push({options,resolve}))};
 vm.createContext(sandbox);vm.runInContext(fs.readFileSync(require('node:path').join(__dirname,'../completion_ui.js'),'utf8')+'\nglobalThis.completion=GoBookCompletion;',sandbox);
 const wrap=new Element(),editor=new Element();editor.value=text;editor.selectionStart=editor.selectionEnd=cursor;editor.focus();
 sandbox.completion.attach(wrap,editor,()=>({token:'test',notebook:{version:1,imports:['fmt'],cells:[{code:editor.value}]},cell:1}));
 const key=props=>editor.dispatchEvent({type:'keydown',preventDefault(){},stopImmediatePropagation(){},...props});
 return {editor,menu:wrap.children[0],requests,key,
  start:()=>key({ctrlKey:true,code:'Space'}),
  async resolve(items=[{label:'Println',insertText:'Println'}]){assert.ok(requests.length,'completion request was sent');requests.at(-1).resolve({ok:true,json:async()=>({provider:'local',items})});await new Promise(setImmediate)},
  async flushTimers(){const pending=[...timers.values()];timers.clear();for(const fn of pending)fn();await new Promise(setImmediate)}};
}

test('Escape dismisses a pending completion without a delayed reopen',async()=>{
 const h=harness();h.start();h.start(); // A second explicit request may queue a retry.
 h.key({key:'Escape'});await h.resolve();await h.flushTimers();
 assert.equal(h.menu.hidden,true);assert.equal(h.requests.length,1,'dismissal also cancels a queued retry');
});

test('completion replaces the whole identifier when the cursor is inside it',async()=>{
 assert.deepEqual(JSON.parse(JSON.stringify(ctx.contextAt('fmt.Println',6))),{prefix:'Pr',start:4,end:11});
 const h=harness('fmt.Println',6);h.start();await h.resolve();
 assert.equal(h.menu.hidden,false,'valid suggestions are displayed');h.key({key:'Enter'});
 assert.equal(h.editor.value,'fmt.Println');assert.equal(h.editor.selectionStart,11);
});

for(const [name,change] of [
 ['selection changes',editor=>{editor.selectionEnd=editor.selectionStart+1}],
 ['editor becomes read-only',editor=>{editor.readOnly=true}],
 ['editor is disconnected',editor=>{editor.isConnected=false}],
])test('pending completion is discarded when '+name,async()=>{
 const h=harness('fmt.Println',6);h.start();change(h.editor);await h.resolve();
 assert.equal(h.menu.hidden,true);assert.equal(h.editor.value,'fmt.Println');
});

test('an unchanged focused editor displays and accepts a completion',async()=>{
 const h=harness();h.start();await h.resolve();assert.equal(h.menu.hidden,false);
 h.key({key:'Enter'});assert.equal(h.editor.value,'fmt.Println');assert.equal(h.menu.hidden,true);
});
