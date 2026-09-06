const key='chaldea.post-auth.route.v2';
const obsoleteKey='chaldea.post-auth.route.v1';
const routes=['/dashboard','/me','/wallet','/wallet/activate','/keys','/logs','/models','/api/access','/playground','/rewards','/games/dice','/master-profile','/account','/admin/channels','/ops/announcements','/ops/models'];
export function validModelID(id:unknown):id is string {return typeof id==='string'&&id.length>0&&id.trim()===id&&!/[\u0000-\u001f\u007f-\u009f]/u.test(id)&&!/[\uD800-\uDFFF]/u.test(id)&&new TextEncoder().encode(id).length<=255;}
// The single extensible navigation whitelist. Future implemented model/access
// routes extend this function, not a second auth client or intent storage key.
export function normalizeRouteIntent(path:string):string|undefined {
 if(routes.includes(path))return path;
 if(path.includes('#'))return;
 const split=path.indexOf('?');if(split<0)return;
 const route=path.slice(0,split),raw=path.slice(split+1);if(!raw||raw.includes(';')||!['/api/access','/keys'].includes(route))return;
 try{decodeURIComponent(raw.replace(/\+/g,' '));}catch{return;}
 const query=new URLSearchParams(raw);if(query.getAll('model_id').length!==1||!validModelID(query.get('model_id')))return;
 for(const key of query.keys())if(key!=='model_id'&&(key!=='intent'||route!=='/api/access'||query.getAll(key).length!==1||query.get(key)!=='use'))return;
 return path;
}
// Only navigation survives the provider round trip. No write body, identity,
// credential or arbitrary destination is retained here.
export function saveRouteIntent(path:string,now=Date.now()) {
    try { sessionStorage.removeItem(obsoleteKey); if(normalizeRouteIntent(path)) sessionStorage.setItem(key,JSON.stringify({route:path,expires:now+30*60*1000})); else sessionStorage.removeItem(key); } catch { /* default dashboard remains available */ }
}
export function peekRouteIntent(now=Date.now()):string {
    try {sessionStorage.removeItem(obsoleteKey);const raw=sessionStorage.getItem(key);if(!raw)return '/dashboard';const data=JSON.parse(raw);if(!data||Object.keys(data).length!==2 || typeof data.route!=='string'||!normalizeRouteIntent(data.route)||!Number.isFinite(data.expires)||data.expires<=now||data.expires>now+30*60*1000)return '/dashboard';return data.route;}catch{return '/dashboard';}
}
export function consumeRouteIntent(now=Date.now()):string {
    const destination=peekRouteIntent(now);try{sessionStorage.removeItem(key);}catch{ /* navigation only */ }return destination;
}
