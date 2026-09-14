import { useEffect } from 'react';
import type { CapturedCallback } from '../Authentication';

export const opsPopupPrefix='CHALDEA_OPS_FRESH_';
export const opsPopupMessage='chaldea-ops-discord-callback';
export function isOpsDiscordPopup(){return !!window.opener&&window.name.startsWith(opsPopupPrefix)}
export function validateOpsDiscordURL(value:unknown){
 if(typeof value!=='string'||value.length>8192)throw new Error('Discord 验证地址无效。');
 let url:URL;try{url=new URL(value)}catch{throw new Error('Discord 验证地址无效。')}
 const query=url.searchParams,keys=['client_id','redirect_uri','response_type','scope','state','prompt'];
 if(url.origin!=='https://discord.com'||url.pathname!=='/oauth2/authorize'||url.username||url.password||url.hash||query.get('redirect_uri')!==window.location.origin+'/oauth/discord'||query.get('response_type')!=='code'||query.get('scope')!=='identify'||query.get('prompt')!=='consent'||!/^\d{1,20}$/.test(query.get('client_id')||'')||!query.get('state')||[...query.keys()].length!==keys.length||keys.some(key=>query.getAll(key).length!==1))throw new Error('Discord 验证地址无效。');
 return url.href;
}
// Callback data was captured and removed from the address bar before React
// mounted. Only the opener at this exact origin can receive this message.
export function OpsDiscordPopup({captured}:{captured:CapturedCallback}){
 useEffect(()=>{if(!isOpsDiscordPopup())return;window.opener.postMessage({type:opsPopupMessage,...(captured.input?{input:captured.input}:{error:'CALLBACK_INVALID'})},window.location.origin)},[captured]);
 return <main className="session-loading"><p role="status">验证结果已返回运营页面。你可以关闭此窗口。</p><button onClick={()=>window.close()}>关闭窗口</button></main>;
}
