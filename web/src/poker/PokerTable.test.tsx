import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PokerTable } from './PokerTable';
import type { PokerTableProps, SeatView } from './poker-ui-types';

const chip=(n:string)=>(BigInt(n)*500000n).toString();
function fixture(): PokerTableProps {
  return {
    authority:{user_id:'u1',runtime_id:'runtime-1',event_sequence:'44',connection_state:'LIVE',has_snapshot:true,pending:false,can_reconnect:false,can_takeover:false,can_control:true,viewer_kind:'PLAYER_SELF'},
    table:{table_id:'AZ-07',name:'月光长廊',lifecycle_state:'IN_HAND',table_version:'8',max_seats:9,small_blind_units:chip('5'),big_blind_units:chip('10'),settings_locked:true,server_now:'2026-09-06T12:00:10Z',timeline:{last_sequence:'0',truncated:false,events:[]},
      seats:[{seat_no:1,display_name:'御主',state:'PLAYING',is_self:true,connected:true,stack_units:chip('780'),street_committed_units:chip('20'),total_committed_units:chip('40'),is_folded:false,is_all_in:false,hole_card_count:2,hole_cards:[0,13],sit_out_next_hand:false,leave_after_hand:false,pending_top_up_units:'0'},
             {seat_no:2,display_name:'Moonlit',state:'PLAYING',is_self:false,connected:true,stack_units:chip('600'),street_committed_units:chip('50'),total_committed_units:chip('80'),is_folded:false,is_all_in:false,hole_card_count:2,sit_out_next_hand:false,leave_after_hand:false,pending_top_up_units:'0'}],
      hand:{hand_id:'hand-1',hand_version:'6',street:'FLOP',button_seat:2,actor_seat:1,board_cards:[28,42,8],pot_units:chip('101'),pots:[{index:0,amount_units:chip('80'),eligible_seats:[1,2],awards:[]},{index:1,amount_units:chip('21'),eligible_seats:[1,2],awards:[]}],action_sequence:'4',action_deadline_at:'2026-09-06T12:00:30Z',recovering:false,server_seed_hash:'hash-only',deck_hash:'hash-only'},
      viewer:{session_id:'s1',seat_no:1,control_epoch:'3',can_act:true,can_top_up:true,can_leave:true,can_resume:false,can_sit_out:true,can_start:false,top_up_min_units:chip('1'),top_up_max_units:chip('200'),legal:{actions:['FOLD','CALL','RAISE','ALL_IN'],to_call_units:chip('30'),call_applied_units:chip('30'),minimum_bet_units:chip('10'),minimum_raise_to_units:chip('80'),maximum_raise_to_units:chip('800'),raise_rights:true,shortcuts:[{name:'1/2 Pot',action_type:'RAISE',target_to_units:chip('115')},{name:'All-in',action_type:'ALL_IN',target_to_units:chip('800')}]}}},
    ui:{bet:{action_type:'RAISE',amount_chips:'80'},top_up_chips:'100',confirm_leave:false,confirm_takeover:false},
    onUiChange:vi.fn(),onIntent:vi.fn(),
  };
}

describe('controlled Poker Table',()=>{
  it.each([false,undefined])('table socket grant %s remains read-only even when every domain action capability is true',grant=>{
    const p=fixture();p.authority.can_control=grant;render(<PokerTable {...p}/>);
    for(const name of ['跟注 30 Chips','下手暂离']){const b=screen.getByRole('button',{name});expect(b).toBeDisabled();fireEvent.click(b);}
    expect(screen.queryByRole('slider')).not.toBeInTheDocument();expect(screen.getByText('当前连接只读')).toBeInTheDocument();expect(p.onIntent).not.toHaveBeenCalled();
  });
  it('a read-only socket keeps explicit owned-session HTTP top-up and safe-leave when domain capabilities allow them',()=>{
    const p=fixture();p.authority.can_control=false;const {rerender}=render(<PokerTable {...p}/>);
    expect(screen.getByRole('button',{name:'提交补充筹码'})).toBeEnabled();fireEvent.click(screen.getByRole('button',{name:'提交补充筹码'}));expect(p.onIntent).toHaveBeenLastCalledWith({type:'topup',amount_units:'50000000'},expect.anything());
    fireEvent.click(screen.getByRole('button',{name:'安全离座'}));expect(p.onUiChange).toHaveBeenLastCalledWith({...p.ui,confirm_leave:true});rerender(<PokerTable {...p} ui={{...p.ui,confirm_leave:true}}/>);fireEvent.click(screen.getByRole('button',{name:'确认安全离座'}));expect(p.onIntent).toHaveBeenLastCalledWith({type:'leave',return_to_lobby:true},expect.anything());
    expect(screen.getByRole('button',{name:'跟注 30 Chips'})).toBeDisabled();expect(screen.getByRole('button',{name:'下手暂离'})).toBeDisabled();expect(p.onIntent).toHaveBeenCalledTimes(2);
  });
  it('offers explicit takeover in the existing LIVE read-only action tray without claiming receipt confirmation is a controller grant',()=>{
    const p=fixture();p.authority={...p.authority,can_control:false,can_takeover:true,ticket_intent:'CLAIM_CONTROL'};const {rerender}=render(<PokerTable {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'申请接管'}));expect(p.onUiChange).toHaveBeenLastCalledWith({...p.ui,confirm_takeover:true});expect(p.onIntent).not.toHaveBeenCalled();
    rerender(<PokerTable {...p} ui={{...p.ui,confirm_takeover:true}}/>);fireEvent.click(screen.getByRole('button',{name:'确认接管牌桌'}));expect(p.onIntent).toHaveBeenCalledWith({type:'takeover'},expect.anything());expect(screen.getByRole('button',{name:'跟注 30 Chips'})).toBeDisabled();
  });
  it('READ_ONLY ticket requests only a new explicit control connection, never automatic takeover',()=>{
    const p=fixture();p.authority={...p.authority,can_control:false,can_takeover:false,ticket_intent:'READ_ONLY'};render(<PokerTable {...p}/>);
    expect(screen.queryByRole('button',{name:'申请接管'})).not.toBeInTheDocument();fireEvent.click(screen.getByRole('button',{name:'申请控制连接'}));expect(p.onIntent).toHaveBeenCalledTimes(1);expect(p.onIntent).toHaveBeenCalledWith({type:'reconnect',control_intent:'CLAIM_CONTROL'},expect.anything());
  });
  it('automatic lifecycle never exposes a public start-hand command even if the domain metadata can_start is true',()=>{
    const p=fixture();p.table.viewer.can_start=true;render(<PokerTable {...p}/>);expect(screen.queryByRole('button',{name:'开始下一手'})).not.toBeInTheDocument();expect(p.onIntent).not.toHaveBeenCalled();
  });
  it('sends exact current legal call/raise intents and uses shortcuts only to edit the draft',()=>{
    const p=fixture();render(<PokerTable {...p}/>);
    expect(screen.queryByRole('button',{name:/^过牌/})).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:'跟注 30 Chips'}));
    expect(p.onIntent).toHaveBeenLastCalledWith({type:'action',action_type:'CALL',target_to_units:'0'},expect.objectContaining({table_version:'8',hand_version:'6',control_epoch:'3',action_sequence:'4'}));
    vi.mocked(p.onIntent).mockClear(); fireEvent.click(screen.getByRole('button',{name:'1/2 Pot'}));
    expect(p.onUiChange).toHaveBeenCalledWith({...p.ui,bet:{action_type:'RAISE',amount_chips:'115'}});
    expect(p.onIntent).not.toHaveBeenCalled();
    expect(screen.getByText(/本轮总额 80 · 本次追加 60 Chips/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button',{name:'确认加注至 80 Chips'}));
    expect(p.onIntent).toHaveBeenLastCalledWith({type:'action',action_type:'RAISE',target_to_units:'40000000'},expect.anything());
  });

  it('renders only own private cards; malicious other hole fields stay hidden, and side pots remain distinct',()=>{
    const p=fixture();p.table.seats[1]={...p.table.seats[1],hole_cards:[12,25]} as unknown as SeatView;
    render(<PokerTable {...p}/>);
    expect(screen.queryByLabelText('K 梅花')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('K 方块')).not.toBeInTheDocument();
    expect(screen.getAllByLabelText('未公开手牌')).toHaveLength(2);
    expect(screen.getByRole('region',{name:'主池'})).toHaveTextContent('80');
    expect(screen.getByRole('region',{name:'边池 1'})).toHaveTextContent('21');
    expect(screen.getAllByRole('listitem',{name:/号座位/})).toHaveLength(9);
  });

  it('shows explicitly server-released showdown cards but keeps folded and unreleased hands hidden',()=>{
    const p=fixture();p.table.seats[1]={...p.table.seats[1],public_hole_cards:[12,25],hole_cards_released:true};
    p.table.hand={...p.table.hand!,street:'SETTLED',actor_seat:0};
    const {rerender}=render(<PokerTable {...p}/>);
    expect(screen.getByLabelText('K 梅花')).toBeInTheDocument();expect(screen.getByLabelText('K 方块')).toBeInTheDocument();
    expect(screen.getByLabelText('服务端行动计时')).toHaveTextContent('本手已结束 · 等待下一手');
    p.table.seats[1]={...p.table.seats[1],is_folded:true};rerender(<PokerTable {...p}/>);
    expect(screen.queryByLabelText('K 梅花')).not.toBeInTheDocument();expect(screen.getAllByLabelText('未公开手牌')).toHaveLength(2);
    p.table.seats[1]={...p.table.seats[1],is_folded:false,hole_cards_released:false};rerender(<PokerTable {...p}/>);
    expect(screen.queryByLabelText('K 方块')).not.toBeInTheDocument();expect(screen.getAllByLabelText('未公开手牌')).toHaveLength(2);
  });

  it.each(['DISCONNECTED','RECONNECTING','TAKEN_OVER','SYNCING'] as const)('makes %s read-only, without retry or simulated timer actions',(state)=>{
    const p=fixture();p.authority={...p.authority,connection_state:state,can_reconnect:true};render(<PokerTable {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'跟注 30 Chips'}));
    expect(p.onIntent).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button',{name:'重新连接'}));
    expect(p.onIntent).toHaveBeenCalledTimes(1);
    expect(p.onIntent).toHaveBeenCalledWith({type:'reconnect'},expect.anything());
  });

  it('uses a paused server recovery state and rejects malformed or out-of-range raise amounts',()=>{
    const p=fixture();p.table.hand={...p.table.hand!,recovering:true};const {rerender}=render(<PokerTable {...p}/>);
    expect(screen.getByText('服务恢复中 · 行动已暂停')).toBeInTheDocument();
    expect(screen.getByRole('button',{name:'跟注 30 Chips'})).toBeDisabled();
    expect(screen.getByRole('button',{name:'下手暂离'})).toBeDisabled();
    expect(screen.getByRole('button',{name:'安全离座'})).toBeDisabled();
    p.table.hand={...p.table.hand!,recovering:false};p.ui={...p.ui,bet:{action_type:'RAISE',amount_chips:'79'}};
    rerender(<PokerTable {...p}/>); expect(screen.getByRole('button',{name:/确认加注至/})).toBeDisabled();
    p.ui={...p.ui,bet:{action_type:'RAISE',amount_chips:'80.5'}};rerender(<PokerTable {...p}/>);
    expect(screen.getByRole('button',{name:/确认加注至/})).toBeDisabled();
    expect(p.onIntent).not.toHaveBeenCalled();
  });

  it('offers an exact integer slider only as a controlled amount edit, not a wager',()=>{
    const p=fixture();render(<PokerTable {...p}/>);
    fireEvent.change(screen.getByRole('slider',{name:'调整本轮下注总额'}),{target:{value:'500'}});
    expect(p.onUiChange).toHaveBeenCalledWith({...p.ui,bet:{action_type:'RAISE',amount_chips:'440'}});
    expect(p.onIntent).not.toHaveBeenCalled();
    expect(screen.getByText('尚需跟注 30 · 本次跟注追加 30 Chips')).toBeInTheDocument();
  });

  it('returns to Lobby through explicit safe-leave intent, never clearing or cashing out the hand locally',()=>{
    const p=fixture();const {rerender}=render(<PokerTable {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'返回大厅'}));
    expect(p.onIntent).not.toHaveBeenCalled();expect(p.onUiChange).toHaveBeenCalledWith({...p.ui,confirm_leave:true});
    rerender(<PokerTable {...p} ui={{...p.ui,confirm_leave:true}}/>);
    fireEvent.click(screen.getByRole('button',{name:'确认安全离座'}));
    expect(p.onIntent).toHaveBeenCalledWith({type:'leave',return_to_lobby:true},expect.anything());
    expect(screen.getByRole('region',{name:'主池'})).toHaveTextContent('80');
  });

  it('does not treat a missing seat during resync as a spectator free to bypass safe leave',()=>{
    const p=fixture();p.table.seats=[];p.authority={...p.authority,connection_state:'SYNCING',has_snapshot:false};
    render(<PokerTable {...p}/>);
    expect(screen.getByRole('button',{name:'返回大厅'})).toBeDisabled();
    fireEvent.click(screen.getByRole('button',{name:'返回大厅'}));
    expect(p.onIntent).not.toHaveBeenCalled();
  });

  it('limits top-up to authority, requires explicit takeover, and expires only from supplied server time',()=>{
    const p=fixture();p.ui={...p.ui,top_up_chips:'201'};const {rerender}=render(<PokerTable {...p}/>);
    expect(screen.getByRole('button',{name:'提交补充筹码'})).toBeDisabled();
    p.ui={...p.ui,top_up_chips:'100'};rerender(<PokerTable {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'提交补充筹码'}));
    expect(p.onIntent).toHaveBeenLastCalledWith({type:'topup',amount_units:'50000000'},expect.anything());
    p.authority={...p.authority,connection_state:'TAKEN_OVER',can_takeover:true};p.ui={...p.ui,confirm_takeover:true};rerender(<PokerTable {...p}/>);
    fireEvent.click(screen.getByRole('button',{name:'确认接管牌桌'}));expect(p.onIntent).toHaveBeenLastCalledWith({type:'takeover'},expect.anything());
    p.authority={...p.authority,connection_state:'LIVE'};p.table={...p.table,server_now:'2026-09-06T12:00:30Z'};rerender(<PokerTable {...p}/>);
    expect(screen.getByRole('button',{name:'跟注 30 Chips'})).toBeDisabled();
  });
});
