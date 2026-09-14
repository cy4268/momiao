import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PokerTable } from './PokerTable';
import { parsePokerTableView } from './poker-view';
import g3Self from './fixtures/g3-player-self.json';

describe('real G3 wire projection in the controlled table',()=>{
  it('renders exactly one authoritative Min shortcut when G3 already supplies Min',()=>{
    const table=parsePokerTableView(g3Self,g3Self.table_id,'PLAYER_SELF');
    render(<PokerTable table={table} authority={{user_id:'910001',runtime_id:'synthetic-runtime',event_sequence:'0',connection_state:'LIVE',has_snapshot:true,pending:false,can_reconnect:false,can_takeover:false,can_control:true,viewer_kind:'PLAYER_SELF'}} ui={{bet:{action_type:'RAISE',amount_chips:'20'},top_up_chips:'1',confirm_leave:false,confirm_takeover:false}} onUiChange={vi.fn()} onIntent={vi.fn()}/>);
    expect(screen.getAllByRole('button',{name:'Min'})).toHaveLength(1);
    expect(screen.getByLabelText('当前行动区')).toHaveTextContent('轮到你行动');
  });
});
