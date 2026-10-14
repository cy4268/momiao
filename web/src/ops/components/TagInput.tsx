import { useId, useState } from 'react';

export function TagInput({label,values,onChange,options=[]}:{label:string;values:string[];onChange:(values:string[])=>void;options?:string[]}) {
 const id=useId(),[draft,setDraft]=useState('');
 function add(text:string) {onChange([...new Set([...values,...text.split(/[\n\r,，]+/).map(value=>value.trim()).filter(Boolean)])]);setDraft('')}
 return <div className="ops-tag-field">
  <label htmlFor={id}>{label}</label>
  <div className="ops-tag-input">{values.map(value=><span className="ops-tag" key={value}>{value}<button type="button" aria-label={`移除 ${label} ${value}`} onClick={()=>onChange(values.filter(item=>item!==value))}>×</button></span>)}
   <input id={id} value={draft} list={options.length?id+'-options':undefined} placeholder="输入后回车，或粘贴多行" onChange={e=>setDraft(e.target.value)} onKeyDown={e=>{if(e.key==='Enter'&&!e.nativeEvent.isComposing){e.preventDefault();add(draft)}}} onPaste={e=>{const text=e.clipboardData.getData('text');if(/[\n\r,，]/.test(text)){e.preventDefault();add(text)}}}/>
  </div>
  {options.length>0&&<datalist id={id+'-options'}>{options.filter(value=>!values.includes(value)).map(value=><option key={value} value={value}/>)}</datalist>}
 </div>;
}
