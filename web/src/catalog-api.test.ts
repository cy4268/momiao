import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import { catalogModelPath, decodeCatalogModelPath, catalogSelectionPath, catalogCurl, decimalDisplay, useCatalogModel } from './catalog-api';
import { peekRouteIntent } from './post-auth-intent';
import { bundle, ok } from './m1-test-fixtures';

it('checks the shared Gate before reading keys and retains a blocked model destination',async()=>{
 sessionStorage.clear();const calls:string[]=[];const navigate=vi.fn();
 const client=new ApiClient(async path=>{calls.push(path);if(path==='/api/user/login')return ok(bundle);if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:'1',route:'/api/access?model_id=team%2Fmodel&intent=use',stage:'MIGRATION_UNVERIFIED'});return ok({items:[],total:0,page:1,page_size:1})});
 await client.login('one','x');await useCatalogModel(client,'team/model',navigate,()=>{throw new Error('already signed in')});
 expect(navigate).toHaveBeenCalledWith('/welcome');expect(peekRouteIntent()).toBe('/api/access?model_id=team%2Fmodel&intent=use');expect(calls.some(path=>path.startsWith('/api/token/'))).toBe(false);
});

it('continues a READY model choice to explicit key creation without creating a key itself',async()=>{
 const calls:string[]=[];const navigate=vi.fn();
 const client=new ApiClient(async path=>{calls.push(path);if(path==='/api/user/login')return ok(bundle);if(path.startsWith('/platform/v1/access-gate?'))return ok({user_id:'1',route:'/api/access?model_id=team%2Fmodel&intent=use',stage:'READY'});return ok({items:[],total:0,page:1,page_size:1})});
 await client.login('one','x');await useCatalogModel(client,'team/model',navigate,()=>{});
 expect(calls.map(path=>path.split('?')[0])).toEqual(['/api/user/login','/platform/v1/access-gate','/api/token/']);expect(navigate).toHaveBeenCalledWith('/keys?model_id=team%2Fmodel');
});

it('round trips opaque IDs without URL dot normalization or double decoding',()=>{
 for(const id of ['model','组织/模型','/','.','..','%2F',"a'$(echo x)",'~Lg','a?b#c','x y']){
  const path=catalogModelPath(id);expect(new URL(path,'https://portal.example').pathname).toBe(path);expect(decodeCatalogModelPath(path)).toBe(id);
 }
 expect(decodeCatalogModelPath('/models/')).toBeNull();expect(decodeCatalogModelPath('/models/%00')).toBeNull();expect(decodeCatalogModelPath('/models/%GG')).toBeNull();
 expect(()=>catalogSelectionPath(' x')).toThrow();expect(catalogSelectionPath('a/b')).toBe('/api/access?model_id=a%2Fb&intent=use');
});
it('keeps explicit zero, unknown and tiny decimal prices distinct',()=>{
 expect(decimalDisplay('0.000')).toBe('0');expect(decimalDisplay(null)).toBe('未提供');expect(decimalDisplay('0.000000000000000001')).toBe('0.000000000000000001');expect(decimalDisplay('12.3400')).toBe('12.34');
});
it('builds only verified endpoints with JSON and POSIX shell quoting and placeholder keys',()=>{
 const id="组织/a'$(echo secret)";const curl=catalogCurl('https://api.example/v1',id,{kind:'openai',path:'/v1/chat/completions',method:'POST'});
 expect(curl).toContain("'https://api.example/v1/chat/completions'");expect(curl).toContain('Bearer <YOUR_API_KEY>');expect(curl).toContain("a'\\''$(echo secret)");expect(curl).toContain('"model":');
 expect(catalogCurl('',id,{kind:'openai',path:'/v1/chat/completions',method:'POST'})).toBe('');
 expect(catalogCurl('https://api.example/v1',id,{kind:'openai',path:'/v1/evil',method:'POST'})).toBe('');
 expect(catalogCurl('https://api.example/v1',id,{kind:'anthropic',path:'/v1/messages',method:'POST'})).toContain('anthropic-version: 2023-06-01');
});
it('reads public catalog without credential headers and discards responses across account changes',async()=>{
 let resolve!:(r:Response)=>void;const pending=new Promise<Response>(r=>resolve=r);
 const fetcher=vi.fn().mockResolvedValueOnce(ok(bundle)).mockReturnValueOnce(pending).mockResolvedValueOnce(ok({...bundle,user:{...bundle.user,id:2},session:{sid:'other'}}));
 const client=new ApiClient(fetcher);await client.login('one','x');const read=client.catalogRequest('/platform/v1/models');const caught=read.catch(e=>e);await client.login('two','x');resolve(ok({items:[]}));expect(await caught).toMatchObject({status:401});
 const headers=new Headers(fetcher.mock.calls[1][1].headers);expect(headers.has('Authorization')).toBe(false);expect(headers.has('New-Api-User')).toBe(false);expect(fetcher.mock.calls[1][1].cache).toBe('no-store');
 await expect(client.catalogRequest('/platform/v1/models/personal-price?model_id=x')).rejects.toThrow();
});
