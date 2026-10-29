const {test}=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');
class Element {
 constructor(){this.children=[];this.hidden=true;this.className='';this.textContent=''}
 get childNodes(){return this.children}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 querySelector(selector){return this.children.find(n=>'.'+n.className===selector)||null}
}
function renderer(){
 const outputs=[new Element(),new Element()],elements={output:new Element(),results:new Element(),cells:{children:outputs.map(output=>({querySelector:()=>output}))}};
 const context={console,WeakMap,Set,document:{createElement:()=>new Element(),querySelectorAll:()=>outputs,querySelector:()=>null,body:{classList:{add(){},remove(){}}}},$:id=>elements[id],validateResult(){},exportText(){},emptyOutput(){},textOutput(label,text,error,parent=elements.output){if(text){const e=new Element();e.textContent=text;parent.append(e)}},renderArtifacts(artifacts,parent){for(const item of artifacts||[]){const e=new Element();e.textContent=item.name;parent.append(e)}}};
 vm.createContext(context);const source=fs.readFileSync(require('node:path').join(__dirname,'../ui.go'),'utf8');
 const start=source.includes('const displayedCellOutputs')?source.indexOf('const displayedCellOutputs'):source.indexOf('function clearCellOutputs');
 vm.runInContext(source.slice(start,source.indexOf('function setBusy',start)),context);
 return {outputs,render:context.renderResult};
}
const result=(a,b,artifacts=[])=>({stdout:a+b,stderr:'',artifacts,cells:[{cell:1,stdout:a,stderr:'',artifacts},{cell:2,stdout:b,stderr:'',artifacts:[]}]});
test('memory output update preserves unchanged cell nodes and replaces changed cell only',()=>{
 const h=renderer();h.render(result('first','second'));const first=h.outputs[0].children[0],second=h.outputs[1].children[0];
 h.render(result('first','changed'),true,true);assert.equal(h.outputs[0].children[0],first);assert.notEqual(h.outputs[1].children[0],second);
});
test('artifact ownership changes refresh its old cell and absent cells are cleared',()=>{
 const h=renderer(),a={name:'model.json',mime:'application/json',data:'e30='};h.render(result('first','second',[a]));const first=h.outputs[0].children[0];
 h.render(result('first','second'),true,true);assert.notEqual(h.outputs[0].children[0],first);
 h.render({stdout:'first',stderr:'',artifacts:[],cells:[{cell:1,stdout:'first',stderr:'',artifacts:[]}]},true,true);assert.equal(h.outputs[1].hidden,true);assert.equal(h.outputs[1].children.length,0);
});

test('a later cell error preserves earlier successful output nodes',()=>{
 const h=renderer();h.render(result('first','second'));const first=h.outputs[0].children[0];
 h.render({...result('first',''),error:'cell 2 failed'},true,true,2);assert.equal(h.outputs[0].children[0],first);
});
