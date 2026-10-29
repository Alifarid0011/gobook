'use strict';
// Rendering is lexical highlighting only. Compilation and debugging remain Go's job.
const GoBookEditor = (() => {
 const tokens = /\/\/[^\n]*|\/\*[\s\S]*?(?:\*\/|$)|"(?:\\[\s\S]|[^"\\])*"?|'(?:\\[\s\S]|[^'\\])*'?|`[^`]*(?:`|$)|\b(?:break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b|\b(?:true|false|nil|iota)\b|\b(?:int|int8|int16|int32|int64|uint|uint8|uint16|uint32|uint64|uintptr|float32|float64|complex64|complex128|bool|string|byte|rune|error|any|comparable|append|make|new|len|cap|copy|clear|delete|close|panic|recover|print|println|min|max|complex|real|imag)\b|\b(?:0[xX][\da-fA-F_]+|\d[\d_]*(?:\.\d[\d_]*)?(?:[eE][+-]?\d[\d_]*)?i?)\b|\b[A-Za-z_]\w*(?=\s*\()/g;
 const keyword = /^(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)$/;
 function attach(wrap, editor) {
  const layer = document.createElement('div'); layer.className = 'code-highlight'; layer.setAttribute('aria-hidden','true');
  const content = document.createElement('div'); content.className = 'highlight-content'; layer.append(content); wrap.prepend(layer);
  const trace = document.createElement('div'); trace.className = 'trace-line'; trace.hidden = true; wrap.prepend(trace);
  let marked = 0, timer, offsets = [0];
  const numbers = wrap.querySelector(".line-numbers");
  function position() {
   content.style.transform = 'translate(' + -editor.scrollLeft + 'px,' + -editor.scrollTop + 'px)';
   const lineHeight = parseFloat(getComputedStyle(editor).lineHeight);
   trace.style.top = (17 + (offsets[marked-1] || 0) - editor.scrollTop) + 'px'; trace.style.height = ((offsets[marked] - offsets[marked-1]) || lineHeight) + 'px';
  }
  function refresh() {
   clearTimeout(timer); const text = editor.value; const frag = document.createDocumentFragment(); let last = 0;
   for (const match of text.matchAll(tokens)) {
    frag.append(document.createTextNode(text.slice(last,match.index))); const span = document.createElement('span'); const value = match[0];
    span.className = value.startsWith('//') || value.startsWith('/*') ? 'syntax-comment' : /^["'`]/.test(value) ? 'syntax-string' : keyword.test(value) ? 'syntax-keyword' : /^\d/.test(value) || /^(true|false|nil|iota)$/.test(value) ? 'syntax-number' : 'syntax-symbol';
    span.textContent = value; frag.append(span); last = match.index + value.length;
   }
   frag.append(document.createTextNode(text.slice(last) + '\n')); content.replaceChildren(frag);
   layer.style.width = editor.clientWidth + 'px';
   const style = getComputedStyle(editor), lineHeight = parseFloat(style.lineHeight), lines = text.split('\n'); offsets = [0];
   if (editor.wrap === 'soft') {
    const ruler = document.createElement('div'); Object.assign(ruler.style,{position:'fixed',left:'-100000px',top:'0',visibility:'hidden',whiteSpace:'pre-wrap',overflowWrap:'anywhere',font:style.font,lineHeight:style.lineHeight,tabSize:style.tabSize,width:(editor.clientWidth-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight))+'px'});
    for (const line of lines) { const row = document.createElement('div'); row.textContent = line || ' '; ruler.append(row); } document.body.append(ruler);
    numbers.replaceChildren(); [...ruler.children].forEach((row,i) => { const height = row.getBoundingClientRect().height; offsets.push(offsets[i]+height); const label = document.createElement('div'); label.textContent = String(i+1); label.style.height = height+'px'; numbers.append(label); }); ruler.remove();
   } else { numbers.textContent = lines.map((_,i)=>i+1).join('\n'); lines.forEach((_,i)=>offsets.push((i+1)*lineHeight)); }
   numbers.scrollTop = editor.scrollTop; position();
  }
  editor.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(refresh,60); });
  editor.addEventListener('scroll', position); new ResizeObserver(refresh).observe(editor); refresh(); wrap.classList.add("highlighted");
  return {refresh, markLine(line) { marked = line; trace.hidden = !line; if (line) { const h = parseFloat(getComputedStyle(editor).lineHeight); editor.scrollTop = Math.max(0,offsets[Math.max(0,line-3)] || 0); } position(); }};
 }
 return {attach};
})();
