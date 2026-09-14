import { useEffect, useState } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { expect, it, vi } from 'vitest';
import { ApiClient } from './api';
import * as Gates from './PostAuthGate';
import { saveRouteIntent } from './post-auth-intent';
import { ok } from './m1-test-fixtures';

const user={id:12,username:'synthetic',display_name:'Fixture',role:1};
const table='/poker/table/550e8400-e29b-41d4-a716-446655440000';
const gate=(route:string,stage:string)=>ok({user_id:'12',route,stage});
async function fixture(){
 const f={sid:'synthetic',reply:(route:string):Promise<Response>=>Promise.resolve(gate(route,'READY'))};
 const fetcher=vi.fn(async(path:string)=>path==='/api/user/login'?ok({access_token:'synthetic',access_expires_at:4102444800,session:{sid:f.sid},user}):path.startsWith('/platform/v1/access-gate?')?f.reply(new URLSearchParams(path.split('?')[1]).get('route')!):ok({enabled:false}));
 const client=new ApiClient(fetcher);await client.login('a','b');return{f,client,fetcher};
}
function probe(){
 const count={mount:0,unmount:0};
 function Client(){
  const policy=Gates.usePokerGatePolicy();
  const [pending]=useState(()=>`original-action-${count.mount+1}`);
  useEffect(()=>{count.mount++;return()=>{count.unmount++;};},[]);
  return <output data-testid="client">{JSON.stringify({...policy,pending})}</output>;
 }
 return{Client,count};
}
const policy=()=>JSON.parse(screen.getByTestId('client').textContent!);
const recheck=()=>fireEvent.click(screen.getByRole('button',{name:'重新核对访问状态'}));

it('keeps the same Poker client and original pending across READY, loading, MAINTENANCE, error and READY',async()=>{
 const{f,client,fetcher}=await fixture();const{Client,count}=probe();
 render(<MemoryRouter><Gates.AccessGate client={client} user={user} route="/poker" pokerRecovery><Client/></Gates.AccessGate></MemoryRouter>);
 await waitFor(()=>expect(policy()).toEqual({stage:'READY',recovery_only:false,mutation_blocked:false,pending:'original-action-1'}));
 let resolve!:(r:Response)=>void;f.reply=()=>new Promise(r=>{resolve=r;});recheck();
 await waitFor(()=>expect(policy()).toEqual({recovery_only:true,mutation_blocked:true,pending:'original-action-1'}));
 expect(count).toEqual({mount:1,unmount:0});await act(async()=>resolve(gate('/poker','MAINTENANCE')));
 await waitFor(()=>expect(policy()).toEqual({stage:'MAINTENANCE',recovery_only:true,mutation_blocked:false,pending:'original-action-1'}));
 f.reply=async()=>{throw new TypeError('synthetic offline');};recheck();await screen.findByText(/访问状态尚未核实/);
 expect(policy()).toEqual({recovery_only:true,mutation_blocked:true,pending:'original-action-1'});expect(count).toEqual({mount:1,unmount:0});
 f.reply=async route=>gate(route,'READY');recheck();await waitFor(()=>expect(policy().stage).toBe('READY'));expect(count).toEqual({mount:1,unmount:0});
 f.reply=async route=>gate(route,'ACCOUNT_RESTRICTED');recheck();await screen.findByText(/账户当前受到访问限制/);await waitFor(()=>{expect(screen.queryByTestId('client')).toBeNull();expect(count.unmount).toBe(1);});
 f.reply=async route=>gate(route,'READY');recheck();await waitFor(()=>{expect(policy().pending).toBe('original-action-2');expect(count).toEqual({mount:2,unmount:1});});
 expect(fetcher.mock.calls.filter(([p])=>!p.startsWith('/platform/v1/access-gate?')).map(([p])=>p)).toEqual(['/api/user/login']);
});

it('drops an old scope and its delayed gate reply rather than restoring its client or permission',async()=>{
 const{f,client}=await fixture();const{Client,count}=probe();
 const tree=(route:string,c=client)=><MemoryRouter><Gates.AccessGate client={c} user={user} route={route} pokerRecovery><Client/></Gates.AccessGate></MemoryRouter>;
 const view=render(tree('/poker'));await waitFor(()=>expect(policy().stage).toBe('READY'));
 let resolve!:(r:Response)=>void;f.reply=()=>new Promise(r=>{resolve=r;});recheck();await waitFor(()=>expect(policy().mutation_blocked).toBe(true));
 f.reply=async route=>gate(route,'MAINTENANCE');view.rerender(tree(table));
 await waitFor(()=>expect(policy()).toEqual({stage:'MAINTENANCE',recovery_only:true,mutation_blocked:false,pending:'original-action-2'}));
 await act(async()=>resolve(gate('/poker','READY')));expect(policy().stage).toBe('MAINTENANCE');expect(count).toEqual({mount:2,unmount:1});
 const other=await fixture();view.rerender(tree(table,other.client));await waitFor(()=>{expect(policy().pending).toBe('original-action-3');expect(count).toEqual({mount:3,unmount:2});});
 const generation=other.client.getSessionGeneration();other.f.sid='synthetic-next';await act(async()=>{await other.client.login('a','b');});expect(other.client.getSessionGeneration()).toBeGreaterThan(generation);
 view.rerender(tree(table,other.client));await waitFor(()=>{expect(policy().pending).toBe('original-action-4');expect(count).toEqual({mount:4,unmount:3});});
});

it('tears down an admitted client as soon as Master is required even when its provisional read fails',async()=>{
 const{f,client,fetcher}=await fixture();const{Client,count}=probe();
 render(<MemoryRouter><Gates.AccessGate client={client} user={user} route="/poker" pokerRecovery><Client/></Gates.AccessGate></MemoryRouter>);
 await waitFor(()=>expect(policy().stage).toBe('READY'));f.reply=async route=>gate(route,'MASTER_REQUIRED');recheck();
 await screen.findByText(/访问状态尚未核实/);await waitFor(()=>{expect(screen.queryByTestId('client')).toBeNull();expect(count).toEqual({mount:1,unmount:1});});
 expect(fetcher.mock.calls.some(([p])=>p==='/platform/v1/admission/ensure')).toBe(false);
});

it('defaults the policy to unverified and blocked outside the explicit Poker provider',()=>{
 const{Client}=probe();render(<Client/>);expect(policy()).toEqual({recovery_only:true,mutation_blocked:true,pending:'original-action-1'});
});

it('admits first Poker maintenance only by explicit slot while other routes and hard gates retain their barriers',async()=>{
 for(const[route,stage,opt,visible]of [[table,'MAINTENANCE',true,true],['/poker','MAINTENANCE',false,false],['/wallet','MAINTENANCE',true,false],['/poker','MIGRATION_UNVERIFIED',true,false],['/poker','ACCOUNT_RESTRICTED',true,false]] as const){
  const{f,client}=await fixture();f.reply=async path=>gate(path,stage);const{Client}=probe();
  const view=render(<MemoryRouter><Gates.AccessGate client={client} user={user} route={route} pokerRecovery={opt}><Client/></Gates.AccessGate></MemoryRouter>);
  await screen.findByRole('button',{name:'重新核对访问状态'});expect(!!screen.queryByTestId('client')).toBe(visible);
  if(visible)expect(policy()).toMatchObject({stage:'MAINTENANCE',recovery_only:true,mutation_blocked:false});view.unmount();
 }
});

it('consumes only canonical maintenance navigation after login without replaying Poker business',async()=>{
 function Destination(){const location=useLocation();return <output data-testid="destination">{location.pathname}</output>;}
 for(const route of ['/poker',table]){
  const{f,client,fetcher}=await fixture();f.reply=async path=>gate(path,'MAINTENANCE');saveRouteIntent(route);
  const view=render(<MemoryRouter initialEntries={['/welcome']}><Routes><Route path="/welcome" element={<Gates.PostAuthGate client={client} user={user}/>}/><Route path="*" element={<Destination/>}/></Routes></MemoryRouter>);
  expect(await screen.findByTestId('destination')).toHaveTextContent(route);expect(sessionStorage.getItem('chaldea.post-auth.route.v2')).toBeNull();
  expect(fetcher.mock.calls.map(([p])=>p)).toEqual(['/api/user/login','/platform/v1/access-gate?route='+encodeURIComponent(route)]);view.unmount();
 }
});
