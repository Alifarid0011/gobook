'use strict';
const GoBookCompletion = (() => {
 let nextID=0;
 function contextAt(text,cursor) {
  if(!Number.isInteger(cursor)||cursor<0||cursor>text.length)return null;
  const before=text.slice(0,cursor),match=before.match(/[\p{L}\p{N}_]*$/u),prefix=match?.[0]||'';
  const suffix=(text.slice(cursor).match(/^[\p{L}\p{N}_]*/u)||[''])[0];return {prefix,start:cursor-prefix.length,end:cursor+suffix.length};
 }
 function attach(wrap,editor,getContext) {
  const menu=document.createElement('div'),list=document.createElement('div'),status=document.createElement('div');
  menu.className='completion-menu';menu.hidden=true;list.id='completion-'+(++nextID);list.setAttribute('role','listbox');list.setAttribute('aria-label','Go code suggestions');status.className='completion-status';status.setAttribute('role','status');menu.append(list,status);wrap.append(menu);
  editor.setAttribute('aria-autocomplete','list');editor.setAttribute('aria-controls',list.id);editor.setAttribute('aria-expanded','false');
  let timer,version=0,inFlight=false,wanted=false,selected=0,items=[],snapshot=null;
  function close(invalidate=true){if(invalidate){version++;clearTimeout(timer);wanted=false}menu.hidden=true;editor.setAttribute('aria-expanded','false');editor.removeAttribute('aria-activedescendant')}
  function select(index){selected=(index+items.length)%items.length;[...list.children].forEach((button,i)=>button.setAttribute('aria-selected',String(i===selected)));const active=list.children[selected];if(active){editor.setAttribute('aria-activedescendant',active.id);active.scrollIntoView({block:'nearest'})}}
  function accept(index=selected){
   if(!snapshot||snapshot.text!==editor.value||snapshot.cursor!==editor.selectionStart||editor.selectionStart!==editor.selectionEnd||editor.readOnly){close();return}
   const item=items[index];if(!item){close();return}
   const start=Number.isInteger(item.start)?item.start:snapshot.range.start,end=Number.isInteger(item.end)?item.end:snapshot.range.end;if(start<0||end<start||end>editor.value.length){close();return}
   editor.setRangeText(item.insertText,start,end,'end');close();version++;editor.dispatchEvent(new Event('input',{bubbles:true}));editor.focus();
  }
  function position(){
   const style=getComputedStyle(editor),lineHeight=parseFloat(style.lineHeight),before=editor.value.slice(0,editor.selectionStart),lines=before.split('\n'),canvas=document.createElement('canvas'),ctx=canvas.getContext('2d');ctx.font=style.font;
   const x=parseFloat(style.paddingLeft)+ctx.measureText(lines.at(-1).replace(/\t/g,' '.repeat(Number(style.tabSize)||4))).width-editor.scrollLeft;
   const y=parseFloat(style.paddingTop)+lines.length*lineHeight-editor.scrollTop;
   menu.style.left=Math.max(42,Math.min(x,Math.max(42,wrap.clientWidth-370)))+'px';menu.style.top=Math.max(24,Math.min(y,editor.clientHeight-70))+'px';
  }
  async function request(force=false){
   clearTimeout(timer);if(editor.readOnly||!editor.isConnected){close();return}
   const range=contextAt(editor.value,editor.selectionStart);if(!range||editor.selectionStart!==editor.selectionEnd||(!force&&!range.prefix&&!editor.value.slice(0,editor.selectionStart).endsWith('.'))){close();return}
   if(inFlight){wanted=true;return}
   let context;try{context=getContext()}catch{close();return}if(!context.token){close();return}
   const revision=version,text=editor.value,cursor=editor.selectionStart;inFlight=true;wanted=false;
   try{
    const response=await fetch('/api/complete',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+context.token},body:JSON.stringify({notebook:context.notebook,cell:context.cell,offset:cursor})});
    if(!response.ok){close();return}const result=await response.json();
    if(revision!==version||text!==editor.value||cursor!==editor.selectionStart||cursor!==editor.selectionEnd||editor.readOnly||!editor.isConnected||document.activeElement!==editor)return;
    items=(result.items||[]).filter(item=>typeof item.label==='string'&&typeof item.insertText==='string').slice(0,40);if(!items.length){close();return}
    snapshot={text,cursor,range};list.replaceChildren();items.forEach((item,i)=>{const button=document.createElement('button'),label=document.createElement('strong'),detail=document.createElement('small');button.type='button';button.id=list.id+'-'+i;button.setAttribute('role','option');label.textContent=item.label;detail.textContent=item.detail||'';button.append(label,detail);button.onpointerdown=e=>{e.preventDefault();accept(i)};list.append(button)});
    status.textContent=(result.provider==='gopls'?'gopls · typed suggestions':'Local suggestions')+' · ↑↓ select · Enter / Tab insert · Esc close';status.title=result.message||'';
    menu.hidden=false;editor.setAttribute('aria-expanded','true');position();select(0);
   }catch{close()}
   finally{inFlight=false;if(wanted){wanted=false;timer=setTimeout(()=>request(true),80)}}
  }
  editor.addEventListener('input',()=>{version++;close();clearTimeout(timer);timer=setTimeout(()=>request(),180)});
  editor.addEventListener('keydown',e=>{
   if((e.ctrlKey||e.metaKey)&&e.code==='Space'){e.preventDefault();e.stopImmediatePropagation();request(true);return}
   if(e.key==='Enter'&&(e.shiftKey||e.ctrlKey||e.metaKey)){close();return}
   if(e.key==='Escape'){e.preventDefault();e.stopImmediatePropagation();close();return}
   if(menu.hidden)return;
   if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();e.stopImmediatePropagation();select(selected+(e.key==='ArrowDown'?1:-1))}
   else if((e.key==='Enter'||e.key==='Tab')&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey){e.preventDefault();e.stopImmediatePropagation();accept()}
   else if(['ArrowLeft','ArrowRight','Home','End','PageUp','PageDown'].includes(e.key))close();
  },true);
  editor.addEventListener('blur',close);editor.addEventListener('scroll',close);editor.addEventListener('click',()=>{version++;close()});
 }
 return {attach,contextAt};
})();
