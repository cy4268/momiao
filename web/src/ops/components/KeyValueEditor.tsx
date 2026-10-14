/*
Copyright (C) 2023-2026 QuantumNous

Adapted from QuantumNous/new-api@f116414284162ad15d8925f7bca494c109b83e93
web/src/features/channels/components/model-mapping-editor.tsx.
This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useId, useRef, useState } from 'react';

type Row={id:number;from:string;to:string};
function parse(json:string):Record<string,string> {
 const value=JSON.parse(json.trim()||'{}');
 if(!value||typeof value!=='object'||Array.isArray(value)||Object.values(value).some(item=>typeof item!=='string'))throw new Error('请输入值为字符串的 JSON 对象');
 return value;
}
export function KeyValueEditor({label,value,onChange,leftLabel='名称',rightLabel='值',options=[]}:{label:string;value:string;onChange:(value:string)=>void;leftLabel?:string;rightLabel?:string;options?:string[]}) {
 const id=useId(),nextId=useRef(0),emitted=useRef<string>(value);
 const [mode,setMode]=useState<'visual'|'json'>('visual'),[json,setJson]=useState(value),[error,setError]=useState('');
 const [rows,setRows]=useState<Row[]>(()=>Object.entries(parse(value)).map(([from,to])=>({id:nextId.current++,from,to})));
 const seen=new Set<string>(),duplicates=new Set<string>();
 for(const row of rows){const key=row.from.trim();if(!key)continue;if(seen.has(key))duplicates.add(key);else seen.add(key)}
 function emit(next:string) {emitted.current=next;onChange(next)}
 function readJson(text:string) {
  try {setRows(Object.entries(parse(text)).map(([from,to])=>({id:nextId.current++,from,to})));setError('');return true}
  catch {setError('请输入值为字符串的 JSON 对象');return false}
 }
 useEffect(()=>{if(value!==emitted.current){setJson(value);readJson(value);emitted.current=value}},[value]);
 function sync(next:Row[]) {
  setRows(next);
  const keys=next.map(row=>row.from.trim()).filter(Boolean),duplicate=new Set(keys).size!==keys.length;
  // Match upstream: invalid JSON prevents saving a duplicate mapping silently.
  const text=duplicate?'{ "duplicate_source_models": ':JSON.stringify(Object.fromEntries(next.filter(row=>row.from.trim()).map(row=>[row.from.trim(),row.to.trim()])),null,2);
  setError(duplicate?'左值重复，请修改标红的项目':'');setJson(text);emit(text);
 }
 return <fieldset className="ops-key-value"><legend>{label}</legend>
  <div className="ops-editor-toolbar" aria-label={label+'编辑模式'}><button type="button" aria-pressed={mode==='visual'} onClick={()=>{if(mode==='json'&&readJson(json))setMode('visual')}}>可视化</button><button type="button" aria-pressed={mode==='json'} onClick={()=>setMode('json')}>JSON</button></div>
  {mode==='json'?<label>{label} JSON<textarea rows={6} value={json} aria-invalid={!!error} onChange={e=>{const text=e.target.value;setJson(text);const valid=readJson(text);emit(valid?text.trim()||'{}':'{ "invalid_json": ')}}/></label>:<>
   {rows.map((row,index)=><div className="ops-key-value-row" key={row.id}>
    <label><span>{leftLabel} {index+1}</span><input list={options.length?id:undefined} value={row.from} aria-invalid={duplicates.has(row.from.trim())} onChange={e=>sync(rows.map(item=>item.id===row.id?{...item,from:e.target.value}:item))}/></label><span aria-hidden="true">→</span>
    <label><span>{rightLabel} {index+1}</span><input value={row.to} onChange={e=>sync(rows.map(item=>item.id===row.id?{...item,to:e.target.value}:item))}/></label>
    <button type="button" aria-label={'删除第 '+(index+1)+' 行'} onClick={()=>sync(rows.filter(item=>item.id!==row.id))}>×</button>
   </div>)}
   <button type="button" onClick={()=>sync([...rows,{id:nextId.current++,from:'',to:''}])}>添加一行</button>
  </>}
  {options.length>0&&<datalist id={id}>{options.map(option=><option key={option} value={option}/>)}</datalist>}
  {error&&<p className="status-error" role="alert">{error}</p>}
 </fieldset>;
}
