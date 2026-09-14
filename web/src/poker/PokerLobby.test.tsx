import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PokerLobby } from './PokerLobby';
import type { PokerLobbyProps } from './poker-ui-types';
import { parsePokerLobbySnapshot } from './poker-lobby-read';
import synthetic from './fixtures/l1-lobby.synthetic.json';

const chip = (n: string) => (BigInt(n) * 500000n).toString();
const tableID=synthetic.tables[0].table_id,sessionID='019a0000-0000-7000-8000-000000000001';
function fixture(): PokerLobbyProps {
  return {
    snapshot:parsePokerLobbySnapshot(synthetic,'910001'),entry_pending:false,
    authority:{scope:{user_id:'910001',session_generation:1,request_generation:1,query_key:'/api/v1/poker'},read_state:'FRESH'},
    filters: { query: '', visibility: 'ALL', open_seats_only: false, max_seats: 'ALL', blind_preset_id: 'ALL', lifecycle_state: 'ALL', spectators_only: false, sort: 'LOW_BLIND',limit:50,cursor:null },
    flow: { kind: 'NONE' },
    onFiltersChange: vi.fn(), onFlowChange: vi.fn(), onCreateDraftChange: vi.fn(), onJoinDraftChange: vi.fn(), onIntent: vi.fn(),
  };
}

describe('controlled Poker Lobby', () => {
  it('soft entry policy blocks creation but preserves reads and spectating',()=>{
    const p:PokerLobbyProps&{entry_blocked:boolean}={...fixture(),entry_blocked:true};render(<PokerLobby {...p}/>);
    expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();expect(screen.getAllByRole('button',{name:/查看座位/})[0]).toBeDisabled();
    expect(screen.getByRole('button',{name:'观战'})).toBeEnabled();expect(screen.getByRole('button',{name:'刷新列表'})).toBeEnabled();
    fireEvent.click(screen.getByRole('button',{name:'创建牌桌'}));expect(p.onFlowChange).not.toHaveBeenCalled();
  });
  it('pending blocks external navigation but permits whole-page refresh and pagination',()=>{
    const p=fixture();p.entry_pending=true;p.snapshot.page.next_cursor='eyJ2IjoxfQ';render(<PokerLobby {...p}/>);
    expect(screen.getByRole('button',{name:'查看钱包 ↗'})).toBeDisabled();expect(screen.getByRole('button',{name:'牌局历史 ↗'})).toBeDisabled();
    expect(screen.getByRole('button',{name:'观战'})).toBeDisabled();expect(screen.getByRole('button',{name:'刷新列表'})).toBeEnabled();
    fireEvent.click(screen.getByRole('button',{name:'刷新列表'}));expect(p.onIntent).toHaveBeenCalledWith({type:'refresh'},p.authority.scope);
    fireEvent.click(screen.getByRole('button',{name:'下一页'}));expect(p.onFiltersChange).toHaveBeenCalledWith({...p.filters,cursor:'eyJ2IjoxfQ'});
  });
  it('IS-12 accepts 40 graphemes without a UTF-16 input cap and blocks the 41st grapheme',()=>{
    const p=fixture();p.flow={kind:'CREATE',draft:{name:'👨‍👩‍👧‍👦'.repeat(40),visibility:'PUBLIC',password:'',max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:false}};
    const {rerender}=render(<PokerLobby {...p}/>);expect(screen.getByLabelText('牌桌名称')).not.toHaveAttribute('maxlength');expect(screen.getByRole('button',{name:'确认创建牌桌'})).toBeEnabled();
    fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));expect(p.onIntent).toHaveBeenCalledWith(expect.objectContaining({name:p.flow.draft.name}),expect.anything());
    p.flow={...p.flow,draft:{...p.flow.draft,name:'👨‍👩‍👧‍👦'.repeat(41)}};rerender(<PokerLobby {...p}/>);expect(screen.getByRole('button',{name:'确认创建牌桌'})).toBeDisabled();
  });
  it.each(['CREATE','JOIN'] as const)('retains IS-12 UTF-8 %s password validation while current password creation stays unavailable',kind=>{
    const p=fixture();if(kind==='JOIN'){p.snapshot.create_options={access_modes:['PUBLIC','PASSWORD'],chat_configurable:true};p.snapshot.tables[1].can_request_access=true;}p.flow=kind==='CREATE'?{kind:'CREATE',draft:{name:'月下',visibility:'PASSWORD',password:'御'.repeat(43),max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:false}}:{kind:'JOIN',table_id:p.snapshot.tables[1].table_id,access_granted:false,can_reserve:false,can_buy_in:false,seats:[],draft:{password:'御'.repeat(43),buy_in_chips:''}};
    const {rerender}=render(<PokerLobby {...p}/>),button=()=>screen.getByRole('button',{name:kind==='CREATE'?'确认创建牌桌':'验证密码'});expect(button()).toBeDisabled();fireEvent.click(button());expect(p.onIntent).not.toHaveBeenCalled();
    p.flow={...p.flow,draft:{...p.flow.draft,password:'御'.repeat(42)+'ab'}} as typeof p.flow;rerender(<PokerLobby {...p}/>);if(kind==='CREATE'){expect(button()).toBeDisabled();}else{expect(button()).toBeEnabled();fireEvent.click(button());expect(p.onIntent).toHaveBeenCalledWith(expect.objectContaining({password:'御'.repeat(42)+'ab'}),p.authority.scope);}
  });
  it('keeps server rows while controlled filter changes await a new whole page', () => {
    const p=fixture(); const { rerender }=render(<PokerLobby {...p}/>);
    expect(screen.getByText('月光长廊')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('搜索牌桌'),{target:{value:'静夜'}});
    expect(p.onFiltersChange).toHaveBeenCalledWith({...p.filters,query:'静夜'});
    expect(screen.getByText('月光长廊')).toBeInTheDocument();
    rerender(<PokerLobby {...p} filters={{...p.filters,query:'静夜'}}/>);
    expect(screen.getByText('月光长廊')).toBeInTheDocument();
    expect(screen.getByText('静夜沙龙')).toBeInTheDocument();
    expect(p.onIntent).not.toHaveBeenCalled();
    p.snapshot.tables.reverse();p.snapshot.page.next_cursor='eyJ2IjoxfQ';rerender(<PokerLobby {...p}/>);expect(screen.getAllByRole('row')[1]).toHaveTextContent('静夜沙龙');
    fireEvent.click(screen.getByRole('button',{name:'下一页'}));expect(p.onFiltersChange).toHaveBeenLastCalledWith({...p.filters,cursor:'eyJ2IjoxfQ'});
    p.filters={...p.filters,cursor:'eyJ2IjoxfQ'};rerender(<PokerLobby {...p}/>);fireEvent.change(screen.getByLabelText('排序'),{target:{value:'NEAR_FULL'}});expect(p.onFiltersChange).toHaveBeenLastCalledWith({...p.filters,sort:'NEAR_FULL',cursor:null});
    fireEvent.change(screen.getByLabelText('每页'),{target:{value:'25'}});expect(p.onFiltersChange).toHaveBeenLastCalledWith({...p.filters,limit:25,cursor:null});
    fireEvent.click(screen.getByRole('button',{name:'首页'}));expect(p.onFiltersChange).toHaveBeenLastCalledWith({...p.filters,cursor:null});fireEvent.click(screen.getByRole('button',{name:'刷新列表'}));expect(p.onIntent).toHaveBeenLastCalledWith({type:'refresh'},p.authority.scope);
    p.filters.sort='NEAR_FULL';p.snapshot.tables.reverse();rerender(<PokerLobby {...p}/>);expect(screen.getAllByRole('row')[1]).toHaveTextContent('月光长廊');
    p.snapshot={...p.snapshot,viewer:{...p.snapshot.viewer,available_chips_units:chip('777')},tables:[p.snapshot.tables[1]],page:{limit:50,next_cursor:null}};rerender(<PokerLobby {...p}/>);expect(screen.queryByText('月光长廊')).not.toBeInTheDocument();expect(screen.getByText('777')).toBeInTheDocument();expect(screen.queryByText('1,200')).not.toBeInTheDocument();expect(screen.getByRole('button',{name:'下一页'})).toBeDisabled();
  });

  it('L2a shows HTTP freshness, never a synthetic WS LIVE label',()=>{
    const p=fixture();const {rerender}=render(<PokerLobby {...p}/>);expect(screen.getByText('HTTP 已更新')).toBeInTheDocument();expect(screen.queryByText('实时连接')).not.toBeInTheDocument();
    for(const read_state of ['LOADING','STALE','ERROR'] as const){rerender(<PokerLobby {...p} authority={{...p.authority,read_state}}/>);expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();expect(screen.getAllByRole('button',{name:/查看座位/})[0]).toBeDisabled();}
    rerender(<PokerLobby {...p} authority={{...p.authority,scope:{...p.authority.scope,user_id:'910002'}}}/>);expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();
  });
  it('L2a blocks unsupported legacy create drafts instead of silently downgrading them',()=>{
    const p=fixture();p.flow={kind:'CREATE',draft:{name:'月下',visibility:'PUBLIC',password:'',max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:true}};
    render(<PokerLobby {...p}/>);expect(screen.getByRole('button',{name:'确认创建牌桌'})).toBeDisabled();fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));expect(p.onIntent).not.toHaveBeenCalled();expect(screen.getByLabelText('开放纯文字聊天')).toBeDisabled();
    const access=within(screen.getByRole('complementary',{name:'创建牌桌表单'})).getByLabelText('访问方式');expect(access).toBeEnabled();expect(access).toHaveValue('PUBLIC');expect(within(access).getAllByRole('option')).toHaveLength(1);
  });

  it('creates only a permitted preset table with no ante or immediate-BB option', () => {
    const p=fixture(); p.flow={kind:'CREATE',draft:{name:'  月光新桌  ',visibility:'PUBLIC',password:'',max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:false}};
    render(<PokerLobby {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'确认创建牌桌'}));
    expect(p.onIntent).toHaveBeenCalledWith({type:'create',name:'月光新桌',visibility:'PUBLIC',max_seats:6,blind_preset_id:'5-10',allow_spectators:true,chat_enabled:false},p.authority.scope);
    expect(screen.queryByLabelText(/Ante|立即付大盲/)).not.toBeInTheDocument();
  });

  it('requires authoritative reservation and respects server buy-in bounds rather than deriving BB multipliers', () => {
    const p=fixture();p.snapshot.tables[0].minimum_buyin_units=chip('500');p.snapshot.tables[0].maximum_buyin_units=chip('900');p.flow={kind:'JOIN',table_id:tableID,access_granted:true,can_reserve:true,can_buy_in:true,seats:[{seat_no:3,available:true}],draft:{seat_no:3,password:'',buy_in_chips:'400'}};
    const {rerender}=render(<PokerLobby {...p}/>);
    expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
    fireEvent.click(screen.getByRole('button',{name:'预留 3 号座位'}));
    expect(p.onIntent).toHaveBeenLastCalledWith({type:'reserve',table_id:tableID,seat_no:3},p.authority.scope);expect(screen.getByText('PG 空座提示：3、4、5、6（非预留）')).toBeInTheDocument();
    p.flow={...p.flow,reservation:{reservation_id:'reservation-1',seat_no:3,expires_at:'2026-09-06T12:00:30Z',valid:true},draft:{...p.flow.draft,buy_in_chips:'499'}};
    rerender(<PokerLobby {...p}/>); expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
    p.flow={...p.flow,draft:{...p.flow.draft,buy_in_chips:'500'}}; rerender(<PokerLobby {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'确认买入并等待大盲'}));
    expect(p.onIntent).toHaveBeenLastCalledWith({type:'buyin',table_id:tableID,reservation_id:'reservation-1',seat_no:3,amount_units:'250000000',entry_mode:'WAIT_FOR_BB'},p.authority.scope);
    fireEvent.click(screen.getByRole('button',{name:'最高买入'}));expect(p.onJoinDraftChange).toHaveBeenLastCalledWith({...p.flow.draft,buy_in_chips:'900'});
    p.flow={...p.flow,draft:{...p.flow.draft,buy_in_chips:'1000.5'}}; rerender(<PokerLobby {...p}/>);
    expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();
  });

  it('locks competing tables for an active session and preserves exact large wallet values', () => {
    const p=fixture();p.snapshot.viewer.available_chips_units='9007199254740993';p.snapshot.viewer.poker_in_play_units=chip('820');
    p.snapshot.active_session={session_id:sessionID,table_id:tableID,table_name:'月光长廊',state:'NEEDS_REVIEW',seat_no:1,stack_units:chip('800'),committed_units:chip('20'),poker_in_play_units:chip('820'),small_blind_units:chip('5'),big_blind_units:chip('10'),ante_units:'0',can_reconnect:true};
    const {rerender}=render(<PokerLobby {...p}/>);
    expect(screen.getByText('18,014,398,509.481986')).toBeInTheDocument();expect(screen.getByRole('region',{name:'当前牌桌会话'})).toHaveTextContent('桌上筹码 800 Chips · 已承诺 20 Chips · 在桌资产 820 Chips · 会话待复核');
    p.snapshot.active_session.state='ACTIVE';rerender(<PokerLobby {...p}/>);expect(screen.getByRole('region',{name:'当前牌桌会话'})).toHaveTextContent('会话进行中');
    expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();
    for (const button of screen.getAllByRole('button',{name:/查看座位/})) expect(button).toBeDisabled();
    fireEvent.click(within(screen.getByRole('region',{name:'当前牌桌会话'})).getByRole('button',{name:'回到当前牌桌'}));
    expect(p.onIntent).toHaveBeenCalledWith({type:'reconnect',table_id:tableID,session_id:sessionID},p.authority.scope);
  });

  it('L2a rechecks selected table can_join rather than retaining an older entry permission',()=>{
    const p=fixture();p.snapshot.tables[0].can_join=false;p.flow={kind:'JOIN',table_id:tableID,access_granted:true,can_reserve:true,can_buy_in:true,seats:[{seat_no:3,available:true}],draft:{seat_no:3,password:'',buy_in_chips:'500'},reservation:{reservation_id:'synthetic-reservation',seat_no:3,expires_at:'2026-09-06T12:01:00Z',valid:true}};
    render(<PokerLobby {...p}/>);expect(screen.getByRole('button',{name:'预留 3 号座位'})).toBeDisabled();expect(screen.getByRole('button',{name:'确认买入并等待大盲'})).toBeDisabled();fireEvent.click(screen.getByRole('button',{name:'预留 3 号座位'}));expect(p.onIntent).not.toHaveBeenCalled();
  });

  it('does not issue mutations while a request is pending or service is in maintenance', () => {
    const p=fixture();p.entry_pending=true;const {rerender}=render(<PokerLobby {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'创建牌桌'}));
    fireEvent.click(screen.getAllByRole('button',{name:/查看座位/})[0]);
    expect(p.onIntent).not.toHaveBeenCalled();expect(p.onFlowChange).not.toHaveBeenCalled();
    p.entry_pending=false;p.snapshot.service.state='MAINTENANCE';p.snapshot.viewer.can_join=false;p.snapshot.viewer.can_create=false;p.snapshot.tables[0].can_join=false;rerender(<PokerLobby {...p}/>);
    expect(screen.getByRole('button',{name:'创建牌桌'})).toBeDisabled();expect(screen.getByRole('button',{name:'观战'})).toBeEnabled();fireEvent.click(screen.getByRole('button',{name:'观战'}));expect(p.onIntent).toHaveBeenLastCalledWith({type:'spectate',table_id:tableID},p.authority.scope);
  });
});
