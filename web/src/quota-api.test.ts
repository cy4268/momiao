import { expect, it } from 'vitest';
import { parseNativeQuota, parseTransfer, parseTransferPending } from './quota-api';
const t={id:'01990000-1111-7777-aaaa-000000000001',user_id:'1',amount:'1',amount_units:'500000',status:'CONFIRMED',reason:'',native_before:'3',native_after:'500003',created_at:'2026-09-05T12:00:00Z',updated_at:'2026-09-05T12:00:01Z'};
it('validates exact native quota and original transfer receipts',()=>{
 expect(parseNativeQuota({user_id:'1',raw_quota:'500003',amount:'1.000006',enabled:true},'1').amount).toBe('1.000006');
 expect(parseTransfer(t,'1').id).toBe(t.id);
 for(const change of [{user_id:'2'},{amount:'1.000001'},{native_after:'500004'},{status:'UNKNOWN'},{amount_units:'-1'}])expect(()=>parseTransfer({...t,...change},'1')).toThrow();
 expect(parseTransferPending(JSON.stringify({key:t.id,amount:'0.000002'}))?.amount).toBe('0.000002');
 expect(()=>parseTransferPending(JSON.stringify({key:t.id,amount:'0.000001'}))).toThrow();
});
