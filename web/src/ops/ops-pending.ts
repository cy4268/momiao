import { opsID } from './ops-api';

// Only correlation metadata is persisted. Passwords, challenges, proofs,
// command input, impact previews and private records stay in memory.
export type OpsPendingMarker={id:string;operation_type:string;path:string};
export const opsPendingKey=(principal:string)=>`chaldea.ops.pending.v1.${principal}`;
export function readOpsPending(key:string):OpsPendingMarker|undefined{
 const raw=localStorage.getItem(key);if(raw===null)return;
 const value=JSON.parse(raw) as OpsPendingMarker;
 if(!value||!opsID(value.id)||typeof value.operation_type!=='string'||!/^[A-Z][A-Z0-9_]{0,127}$/.test(value.operation_type)||typeof value.path!=='string'||!/^\/ops(?:\/[a-z0-9-]+)*$/.test(value.path))throw new Error('原操作恢复信息无法读取，请从操作记录核对；当前不允许发起新的写入。');
 return value;
}
export function saveOpsPending(key:string,value:OpsPendingMarker){
 const previous=readOpsPending(key);if(previous&&previous.id!==value.id)throw new Error(`请先核对原操作 ${previous.id}，再进行新的管理操作。`);
 localStorage.setItem(key,JSON.stringify(value));if(readOpsPending(key)?.id!==value.id)throw new Error('无法保存原操作编号，本次写入未发出。');
}
export function clearOpsPending(key:string,id:string){if(readOpsPending(key)?.id===id)localStorage.removeItem(key);}
