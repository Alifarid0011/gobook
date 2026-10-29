'use strict';
(() => {
 const panel=$('cell-graph'), status=$('graph-status'), canvas=$('graph-canvas');
 const ns='http://www.w3.org/2000/svg';
 function svgElement(name,attributes={},text) { const el=document.createElementNS(ns,name); for(const [key,value] of Object.entries(attributes)) el.setAttribute(key,String(value)); if(text!==undefined) el.textContent=text; return el; }
 function goToCell(number) { const cell=$('cells').children[number-1]; if(!cell)return;setView('both');cell.scrollIntoView({block:'center'});cell.querySelector('textarea').focus({preventScroll:true}); }
 function render(graph) {
  canvas.replaceChildren(); $('graph-edges').replaceChildren();
  const height=Math.max(130,graph.nodes.length*104+24),svg=svgElement('svg',{viewBox:'0 0 900 '+height,role:'img','aria-label':'Static dependency graph between Go cells'});
  const defs=svgElement('defs'),marker=svgElement('marker',{id:'cell-graph-arrow',viewBox:'0 0 10 10',refX:9,refY:5,markerWidth:6,markerHeight:6,orient:'auto-start-reverse'});marker.append(svgElement('path',{d:'M 0 0 L 10 5 L 0 10 z',fill:'currentColor'}));defs.append(marker);svg.append(defs);
  graph.edges.forEach((edge,i)=>{
   const from=(edge.from-1)*104+52,to=(edge.to-1)*104+52,left=24+(i%12)*13;
   const path=svgElement('path',{d:'M 230 '+from+' C '+left+' '+from+', '+left+' '+to+', 230 '+to,fill:'none','stroke-width':1.8,'marker-end':'url(#cell-graph-arrow)'});path.append(svgElement('title',{},'Cell '+edge.from+' → '+edge.to+': '+edge.names.join(', ')));svg.append(path);
   const row=document.createElement('li');const button=document.createElement('button');button.textContent='Cell '+edge.from+' → '+edge.to;button.onclick=()=>goToCell(edge.to);const label=document.createElement('span');label.textContent=' '+edge.names.join(', ');row.append(button,label);$('graph-edges').append(row);
  });
  for(const node of graph.nodes) {
   const y=(node.cell-1)*104+14,group=svgElement('g',{tabindex:0,role:'button','aria-label':'Go to cell '+node.cell});
   group.append(svgElement('rect',{x:230,y,width:640,height:78,rx:8}));group.append(svgElement('text',{x:250,y:y+26,class:'graph-node-title'},'Cell '+String(node.cell).padStart(2,'0')));
   const label='Defines: '+(node.defines.join(', ')||'—');group.append(svgElement('text',{x:250,y:y+50,class:'graph-node-detail'},label.length>83?label.slice(0,80)+'…':label));group.append(svgElement('title',{},'Reads: '+(node.reads.join(', ')||'—')+'; writes: '+(node.writes.join(', ')||'—')));
   group.onclick=()=>goToCell(node.cell);group.onkeydown=event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();goToCell(node.cell)}};svg.append(group);
  }
  canvas.append(svg);$('graph-warning').textContent=graph.warnings.join(' ');
  if(!graph.edges.length){const li=document.createElement('li');li.textContent='No lexical dependencies found. This does not prove that cells are safe to run in parallel.';$('graph-edges').append(li)}
 }
 async function refresh() {
  if(!token){status.textContent='Connect a host to analyze cells.';return}
  let snapshot;try{snapshot=JSON.stringify(notebook())}catch(error){status.textContent=error.message;return}
  $('graph-refresh').disabled=true;status.textContent='Reading variable definitions and uses…';
  try {
   const response=await fetch('/api/analyze',{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:snapshot});
   if(!response.ok)throw Error(await response.text());const graph=await response.json();render(graph);
   status.textContent=JSON.stringify(notebook())===snapshot?graph.nodes.length+' cells · '+graph.edges.length+' dependency links · static analysis only':'Source changed during analysis. Refresh the graph.';
  }catch(error){status.textContent=error.message}finally{$('graph-refresh').disabled=false}
 }
 $('graph-toggle').onclick=()=>{panel.hidden=!panel.hidden;$('graph-toggle').setAttribute('aria-expanded',String(!panel.hidden));if(!panel.hidden){setView('both');refresh();panel.scrollIntoView({block:'nearest'})}};
 $('graph-refresh').onclick=refresh;$('graph-close').onclick=()=>{panel.hidden=true;$('graph-toggle').setAttribute('aria-expanded','false')};
 $('cells').addEventListener('input',()=>{if(!panel.hidden)status.textContent='Source changed. Refresh the graph.'});
})();
