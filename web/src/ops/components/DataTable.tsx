import { useState, type ReactNode } from 'react';
import { Pager } from '../../ui';

const pageSizes=[10,20,50,100];
export function usePageSize(storageKey:string) {
 const [size,setSize]=useState(()=>{
  try {const saved=Number(localStorage.getItem(storageKey));return pageSizes.includes(saved)?saved:10}
  catch {return 10}
 });
 return [size,(value:number)=>{setSize(value);try {localStorage.setItem(storageKey,String(value))} catch {}}] as const;
}

export type Column<T>={key:string;label:ReactNode;render:(row:T)=>ReactNode};
type Props<T>={
 columns:Column<T>[];rows:T[];rowKey:(row:T)=>string;
 page:number;total:number;pageSize:number;onPageChange:(page:number)=>void;onPageSizeChange:(size:number)=>void;
 selected?:string[];onSelectionChange?:(keys:string[])=>void;disabled?:boolean;emptyText?:string;
 onRowClick?:(row:T)=>void;
};
export function DataTable<T>({columns,rows,rowKey,page,total,pageSize,onPageChange,onPageSizeChange,selected=[],onSelectionChange,disabled=false,emptyText='暂无符合条件的数据',onRowClick}:Props<T>) {
 const keys=rows.map(rowKey),count=keys.filter(key=>selected.includes(key)).length;
 return <div className="ops-data-table">
  <div className="table-wrap"><table><thead><tr>
   {onSelectionChange&&<th><input type="checkbox" aria-label="选择当前页" checked={keys.length>0&&count===keys.length} ref={node=>{if(node)node.indeterminate=count>0&&count<keys.length}} disabled={disabled||!keys.length} onChange={e=>onSelectionChange(e.target.checked?[...new Set([...selected,...keys])]:selected.filter(key=>!keys.includes(key)))}/></th>}
   {columns.map(col=><th key={col.key} scope="col">{col.label}</th>)}
  </tr></thead><tbody>
   {rows.map(row=>{const key=rowKey(row);return <tr key={key} className={onRowClick?'ops-clickable-row':undefined} onClick={onRowClick?e=>{if(!(e.target as Element).closest('a,button,input,select,textarea,summary'))onRowClick(row)}:undefined}>
    {onSelectionChange&&<td><input type="checkbox" aria-label={'选择 '+key} checked={selected.includes(key)} disabled={disabled} onChange={e=>onSelectionChange(e.target.checked?[...selected,key]:selected.filter(value=>value!==key))}/></td>}
    {columns.map(col=><td key={col.key}>{col.render(row)}</td>)}
   </tr>})}
   {!rows.length&&<tr><td colSpan={columns.length+(onSelectionChange?1:0)} className="ops-table-empty">{emptyText}</td></tr>}
  </tbody></table></div>
  <div className="ops-table-pagination"><label>每页条数<select value={pageSize} disabled={disabled} onChange={e=>onPageSizeChange(Number(e.target.value))}>{pageSizes.map(size=><option key={size} value={size}>{size}</option>)}</select></label><Pager page={page} total={total} size={pageSize} onChange={onPageChange} disabled={disabled}/></div>
 </div>;
}
