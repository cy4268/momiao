const key='chaldea.post-auth.route.v2';
const obsoleteKey='chaldea.post-auth.route.v1';
const routes=['/dashboard','/me','/wallet','/wallet/activate','/keys','/logs','/models','/api/access','/playground','/rewards','/games/dice','/games/scratch','/games/summon','/games/slot','/games/blackjack','/history','/master-profile','/account','/account/security','/admin/channels','/ops/announcements','/ops/models'];
export function pokerRouteIntent(path:string):boolean {return path==='/poker'||path.length===49&&/^\/poker\/table\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(path);}
export function rouletteRouteIntent(path:string):boolean{return path==='/roulette/devil-roulette'||path==='/roulette/pressure-roulette'||/^\/roulette\/rooms\/[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(path);}
export function validModelID(id:unknown):id is string {return typeof id==='string'&&id.length>0&&id.trim()===id&&!/[\u0000-\u001f\u007f-\u009f]/u.test(id)&&!/[\uD800-\uDFFF]/u.test(id)&&new TextEncoder().encode(id).length<=255;}
// The single extensible navigation whitelist. Future implemented model/access
// routes extend this function, not a second auth client or intent storage key.
export function normalizeRouteIntent(path:string):string|undefined {
 if(['/ops','/ops/games','/ops/poker','/ops/economy','/ops/rewards','/ops/rankings','/ops/users','/ops/records','/ops/support-cases','/ops/incidents','/ops/maintenance','/ops/service-health','/ops/jobs','/ops/attention','/ops/operations','/ops/audit','/ops/access-control'].includes(path)||/^\/ops\/(?:operations|audit)\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(path))return path;
 if(path==='/rankings'||path==='/logs?purpose=ROLEPLAY')return path;
 if(routes.includes(path)||pokerRouteIntent(path)||rouletteRouteIntent(path))return path;
 if(/^\/history\/roulette\/[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(path))return path;
 if(/^\/history\/[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(path))return path;
 if(/^\/(?:history\/(?:rounds|sessions|hands)|wallet\/transactions)\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(path))return path;
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
