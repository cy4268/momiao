import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from 'react';
import type { ApiClient } from '../api';
import type { PokerGatePolicy } from '../PostAuthGate';
import type { LivePokerTableProps } from './LivePokerTable';
import { PokerLobby } from './PokerLobby';
import { capturePokerLobbyScope, readPokerHttpLobby } from './poker-lobby-read';
import { capturePokerEntryScope, currentPokerEntryScope, readPokerEntryReceipt, readPokerReservation, submitPokerEntry, verifyPokerTableAccess, type PokerEntryCommand, type PokerEntryScope, type PokerReservationLookup } from './poker-entry-http';
import { chipInput, chipText, pokerUnits, units, type LobbyFilters, type LobbyFlow, type LobbyIntent, type PokerLobbyAuthority, type PokerLobbyReadScope } from './poker-ui-types';
import type { PokerReceipt } from './poker-wire';
export interface LivePokerLobbyProps {
    client: ApiClient;
    policy: PokerGatePolicy;
    onEnter: (tableID: string, admission: LivePokerTableProps['admission']) => void;
    onNavigate: (destination: 'wallet' | 'history' | 'rewards') => void;
    onPendingChange: (pending: boolean) => void;
}
type Admission = LivePokerTableProps['admission'];
interface Operation {
    command: PokerEntryCommand;
    scope: PokerEntryScope;
    phase: 'SENT' | 'UNKNOWN' | 'ACKNOWLEDGED' | 'RESOLVED';
    receipt?: PokerReceipt;
    seat?: number;
    recoveryAttempt: number;
    recoveryTimer?: ReturnType<typeof setTimeout>;
    recoveryDeferred?: boolean;
    recoveryRead?: number;
}
interface Lease {
    table: string;
    id: string;
    seat: number;
    result: PokerReservationLookup;
    received: number;
    remaining: number;
}
interface Owner {
    api: ApiClient;
    key: string;
    scope: PokerEntryScope;
    alive: boolean;
    readGeneration: number;
    nextReadGeneration: number;
    reading?: number;
    read?: PokerLobbyReadScope;
    accepted?: Admission;
    readState: PokerLobbyAuthority['read_state'];
    filters: LobbyFilters;
    filtered: boolean;
    flow: LobbyFlow;
    notice: string;
    backgroundNotice?: string;
    operation?: Operation;
    busy: boolean;
    lease?: Lease;
    leaseReading?: Lease;
    leaseNotice?: string;
    entered: boolean;
}
const defaults = (): LobbyFilters => ({ query: '', visibility: 'ALL', open_seats_only: false, max_seats: 'ALL', blind_preset_id: 'ALL', lifecycle_state: 'ALL', spectators_only: false, sort: 'LOW_BLIND', limit: 50, cursor: null });
const lobbyRefreshMS = 30_000;
const receiptBackoffMS = [1_000, 2_000, 4_000, 8_000, 15_000, 30_000] as const;
const pending = (o: Owner) => !!o.operation && o.operation.phase !== 'RESOLVED';
const remaining = (lease: Lease | undefined) => lease ? Math.max(0, lease.remaining - (performance.now() - lease.received)) : 0;
const readMatches = (a: PokerLobbyReadScope, b: PokerLobbyReadScope) => a.user_id === b.user_id && a.session_generation === b.session_generation && a.request_generation === b.request_generation && a.query_key === b.query_key;
export function LivePokerLobby(p: LivePokerLobbyProps) {
    const auth = useSyncExternalStore(p.client.subscribe, p.client.getSnapshot, p.client.getSnapshot), generation = p.client.getSessionGeneration();
    const key = auth.ready && !auth.loggingOut && auth.user ? JSON.stringify([auth.user.id, generation]) : null;
    const [owned, setOwned] = useState<Owner | null>(null), [, tick] = useState(0), ownerRef = useRef<Owner | null>(null), latest = useRef(p);
    useLayoutEffect(() => { latest.current = p; });
    const current = (o: Owner | null): o is Owner => !!o && o.alive && ownerRef.current === o && latest.current.client === o.api && currentPokerEntryScope(o.api, o.scope, () => o.scope);
    const entryCurrent = (o: Owner, op: Operation) => current(o) && o.operation === op && currentPokerEntryScope(o.api, op.scope, () => o.scope);
    const policyReady = () => latest.current.policy.stage === 'READY' && !latest.current.policy.mutation_blocked && !latest.current.policy.recovery_only;
    const fresh = (o: Owner, context?: PokerLobbyReadScope) => current(o) && o.readState === 'FRESH' && !!o.accepted && o.read === o.accepted.scope && (!context || readMatches(context, o.read));
    function show(o: Owner) { if (current(o))
        tick(n => n + 1); }
    function stopRecovery(op: Operation | undefined) {
        if (op?.recoveryTimer !== undefined) {
            clearTimeout(op.recoveryTimer);
            delete op.recoveryTimer;
        }
        if (op)
            delete op.recoveryDeferred;
    }
    const recoveryAvailable = () => (typeof document === 'undefined' || document.visibilityState === 'visible') && (typeof navigator === 'undefined' || navigator.onLine);
    function scheduleRecovery(o: Owner, op: Operation) {
        if (!entryCurrent(o, op) || !pending(o) || op.recoveryTimer !== undefined || op.recoveryDeferred)
            return;
        const delay = receiptBackoffMS[Math.min(op.recoveryAttempt, receiptBackoffMS.length - 1)];
        op.recoveryAttempt++;
        if (!recoveryAvailable()) {
            op.recoveryDeferred = true;
            return;
        }
        op.recoveryTimer = setTimeout(() => {
            delete op.recoveryTimer;
            if (!recoveryAvailable()) {
                op.recoveryDeferred = true;
                return;
            }
            if (entryCurrent(o, op) && pending(o))
                void recover(o, op);
        }, delay);
    }
    function wakeRecovery(o: Owner) {
        const op = o.operation;
        if (!recoveryAvailable() || !op || !entryCurrent(o, op) || !pending(o) || o.busy)
            return;
        stopRecovery(op);
        void recover(o, op);
    }
    function clearBackgroundNotice(o: Owner) {
        const notice = o.backgroundNotice;
        delete o.backgroundNotice;
        if (notice !== undefined && o.notice === notice)
            o.notice = '';
    }
    function clearLeaseNotice(o: Owner) {
        const notice = o.leaseNotice;
        delete o.leaseNotice;
        if (notice !== undefined && o.notice === notice)
            o.notice = '';
    }
    async function read(o: Owner, background = false): Promise<Admission | undefined> {
        if (!current(o) || background && o.reading !== undefined)
            return;
        const request = ++o.nextReadGeneration;
        o.readGeneration = request;
        o.reading = request;
        try {
            const query = o.filtered ? { ...o.filters } : undefined, scope = capturePokerLobbyScope(o.api, query, request);
            if (!background) {
                o.read = scope;
                o.readState = 'LOADING';
                show(o);
            }
            const getCurrent = () => current(o) && o.readGeneration === request ? scope : null;
            const snapshot = await readPokerHttpLobby(o.api, scope, getCurrent, query);
            if (!snapshot || !current(o) || o.readGeneration !== request)
                return;
            o.read = scope;
            o.accepted = { snapshot, scope };
            o.readState = 'FRESH';
            clearBackgroundNotice(o);
            const op = o.operation;
            if (op?.phase === 'ACKNOWLEDGED' && op.receipt)
                settleFromAdmission(o, op, o.accepted);
            show(o);
            return o.accepted;
        }
        catch {
            if (current(o) && o.readGeneration === request) {
                if (!background) {
                    o.readState = 'ERROR';
                    delete o.backgroundNotice;
                    o.notice = '大厅读取尚未完成，请重新刷新。';
                }
                else {
                    const notice = '后台刷新暂未完成，继续保留最近一次完整大厅状态。';
                    if (!o.notice || o.notice === o.backgroundNotice) {
                        o.notice = notice;
                        o.backgroundNotice = notice;
                    }
                }
                show(o);
            }
        }
        finally {
            if (current(o) && o.reading === request)
                delete o.reading;
        }
    }
    useEffect(() => {
        if (!key)
            return;
        let scope: PokerEntryScope;
        try {
            scope = capturePokerEntryScope(p.client, 0);
        }
        catch {
            return;
        }
        const o: Owner = { api: p.client, key, scope, alive: true, readGeneration: 0, nextReadGeneration: 0, readState: 'LOADING', filters: defaults(), filtered: false, flow: { kind: 'NONE' }, notice: '', busy: false, entered: false };
        ownerRef.current = o;
        setOwned(o);
        void read(o);
        return () => { stopRecovery(o.operation); if (o.operation)
            delete o.operation.recoveryRead; o.readGeneration = ++o.nextReadGeneration; o.alive = false; if (ownerRef.current === o)
            ownerRef.current = null; };
    }, [p.client, key]);
    useEffect(() => { if (!owned)
        return; const timer = setInterval(() => show(owned), 1000); return () => clearInterval(timer); }, [owned]);
    useEffect(() => {
        if (!owned)
            return;
        const refresh = () => {
            if (recoveryAvailable() && current(owned)) {
                void read(owned, true);
                void refreshLease(owned);
            }
        };
        const wake = () => {
            if (!recoveryAvailable() || !current(owned))
                return;
            wakeRecovery(owned);
            refresh();
        };
        const visible = () => { if (document.visibilityState === 'visible')
            wake(); };
        window.addEventListener('focus', wake);
        window.addEventListener('online', wake);
        document.addEventListener('visibilitychange', visible);
        const timer = setInterval(refresh, lobbyRefreshMS);
        return () => { clearInterval(timer); window.removeEventListener('focus', wake); window.removeEventListener('online', wake); document.removeEventListener('visibilitychange', visible); };
    }, [owned]);
    function permitted(o: Owner, command?: PokerEntryCommand) {
        if (!fresh(o) || !policyReady())
            return false;
        const s = o.accepted!.snapshot;
        if (s.service.state !== 'READY' || !s.service.production_ready || !s.viewer.profile_complete || !s.viewer.can_join || s.active_session)
            return false;
        if (!command)
            return true;
        if (command.kind === 'create')
            return s.viewer.can_create && s.viewer.owned_open_table_id === null && s.blind_presets.some(p => p.id === command.blind_preset) &&
                s.create_options.access_modes.includes(command.access_mode) && (!command.chat_enabled || s.create_options.chat_configurable) &&
                (command.access_mode === 'PUBLIC' ? command.password === undefined : typeof command.password === 'string' && command.password.length > 0 && new TextEncoder().encode(command.password).length <= 128);
        const row = s.tables.find(t => t.table_id === command.table_id);
        if (!row || !row.can_join || !row.accepting_players)
            return false;
        if (command.kind === 'reserve')
            return row.open_seat_numbers.includes(command.seat_no) && row.occupied_seats < row.max_seats;
        const amount = pokerUnits(command.amount_units), min = pokerUnits(row.minimum_buyin_units), max = pokerUnits(row.maximum_buyin_units), wallet = units(s.viewer.available_chips_units);
        return amount !== undefined && amount > 0n && min !== undefined && max !== undefined && wallet !== undefined && amount >= min && amount <= max && amount <= wallet;
    }
    function join(o: Owner, table: string) {
        const row = o.accepted?.snapshot.tables.find(t => t.table_id === table);
        if (!row)
            return false;
        o.flow = { kind: 'JOIN', table_id: table, access_granted: row.visibility === 'PUBLIC' || row.can_join || row.can_spectate, can_reserve: row.can_join, can_buy_in: false, seats: [], draft: { password: '', buy_in_chips: chipText(row.minimum_buyin_units) } };
        return true;
    }
    function finish(o: Owner, op: Operation) {
        if (!entryCurrent(o, op) || op.phase === 'RESOLVED')
            return;
        op.phase = 'RESOLVED';
        stopRecovery(op);
        delete op.recoveryRead;
        if(op.command.kind==='create'&&op.command.password!==undefined)delete op.command.password;
        o.busy = false;
        latest.current.onPendingChange(false);
        show(o);
    }
    async function queryLease(o: Owner, table: string, id: string, seat: number, expected?: Lease) {
        const start = performance.now(), scope = { ...o.scope };
        const targetCurrent = () => current(o) && (!expected || o.lease === expected && !pending(o));
        const result = await readPokerReservation(o.api, scope, () => targetCurrent() ? o.scope : null, table, id);
        if (!result || !targetCurrent())
            return;
        if (result.state === 'FOUND' && result.reservation.seat_no !== seat)
            throw Error('reservation seat mismatch');
        const received = performance.now();
        o.lease = { table, id, seat, result, received, remaining: result.state === 'FOUND' && result.reservation.valid ? Math.max(0, Date.parse(result.reservation.expires_at) - Date.parse(result.reservation.checked_at) - (received - start)) : 0 };
        clearLeaseNotice(o);
        if (o.flow.kind === 'JOIN' && o.flow.table_id === table) {
            const r = result.state === 'FOUND' ? result.reservation : undefined;
            o.flow = { ...o.flow, draft: { ...o.flow.draft, seat_no: seat }, reservation: r ? { reservation_id: id, seat_no: seat, expires_at: r.expires_at, valid: r.valid } : undefined };
        }
        show(o);
        return result;
    }
    function targetRead(o: Owner, table?: string) {
        o.filters = table ? { ...defaults(), query: table } : defaults();
        o.filtered = !!table;
        return read(o);
    }
    async function readOperationAdmission(o: Owner, op: Operation, table?: string): Promise<Admission | undefined> {
        if (!entryCurrent(o, op) || !pending(o))
            return;
        const query = table ? { ...defaults(), query: table } : undefined;
        const request = ++o.nextReadGeneration, scope = capturePokerLobbyScope(o.api, query, request);
        op.recoveryRead = request;
        const getCurrent = () => entryCurrent(o, op) && pending(o) && op.recoveryRead === request ? scope : null;
        const snapshot = await readPokerHttpLobby(o.api, scope, getCurrent, query);
        if (!snapshot || !getCurrent())
            return;
        return { snapshot, scope };
    }
    function settleFromAdmission(o: Owner, op: Operation, accepted: Admission) {
        if (!entryCurrent(o, op) || op.phase !== 'ACKNOWLEDGED' || !op.receipt)
            return false;
        if (op.command.kind === 'create') {
            if (!accepted.snapshot.tables.some(row => row.table_id === op.receipt!.table_id))
                return false;
            o.filters = { ...defaults(), query: op.receipt.table_id };
            o.filtered = true;
            o.readGeneration = accepted.scope.request_generation;
            o.read = accepted.scope;
            o.accepted = accepted;
            o.readState = 'FRESH';
            clearBackgroundNotice(o);
            join(o, op.receipt.table_id);
            finish(o, op);
            return true;
        }
        if (op.command.kind !== 'buyin' || op.receipt.status !== 'CONFIRMED')
            return false;
        const active = accepted.snapshot.active_session;
        if (active?.table_id !== op.command.table_id || active.session_id !== op.receipt.session_id || !active.can_reconnect || o.entered)
            return false;
        o.entered = true;
        finish(o, op);
        if (current(o))
            latest.current.onEnter(op.command.table_id, accepted);
        return true;
    }
    async function acceptReceipt(o: Owner, op: Operation, receipt: PokerReceipt) {
        if (!entryCurrent(o, op))
            return;
        op.receipt = receipt;
        op.phase = 'ACKNOWLEDGED';
        op.recoveryAttempt = 0;
        stopRecovery(op);
        if (op.command.kind === 'create') {
            o.filters = { ...defaults(), query: receipt.table_id };
            o.filtered = true;
        }
        show(o);
        if (op.command.kind === 'create') {
            const accepted = await readOperationAdmission(o, op, receipt.table_id);
            if (accepted)
                settleFromAdmission(o, op, accepted);
        }
        else if (op.command.kind === 'reserve') {
            const result = await queryLease(o, op.command.table_id, receipt.reservation_id!, op.command.seat_no);
            if (result?.state === 'FOUND')
                finish(o, op);
        }
        else if (receipt.status === 'FAILED_NO_EFFECT') {
            finish(o, op);
            await read(o);
        }
        else {
            const accepted = await readOperationAdmission(o, op);
            if (accepted)
                settleFromAdmission(o, op, accepted);
        }
        if (entryCurrent(o, op) && pending(o)) {
            o.notice = '原回执已确认，系统将继续自动核对当前入桌条件。';
            show(o);
            scheduleRecovery(o, op);
        }
    }
    async function send(o: Owner, op: Operation) {
        try {
            const receipt = await submitPokerEntry(o.api, op.scope, () => current(o) ? o.scope : null, op.command, policyReady);
            if (receipt && entryCurrent(o, op))
                await acceptReceipt(o, op, receipt);
        }
        catch {
            if (entryCurrent(o, op)) {
                if (!op.receipt)
                    op.phase = 'UNKNOWN';
                o.notice = '原操作结果尚未核实；系统将自动只读查询原回执，不会重发。';
                show(o);
            }
        }
        finally {
            if (entryCurrent(o, op)) {
                o.busy = false;
                show(o);
                if (pending(o))
                    scheduleRecovery(o, op);
            }
        }
    }
    async function recover(o: Owner, op: Operation) {
        if (!entryCurrent(o, op) || !pending(o))
            return;
        if (o.busy) {
            scheduleRecovery(o, op);
            return;
        }
        o.busy = true;
        show(o);
        try {
            if (op.receipt) {
                if (op.command.kind === 'reserve') {
                    const result = await queryLease(o, op.command.table_id, op.receipt.reservation_id!, op.command.seat_no);
                    if (result?.state === 'FOUND')
                        finish(o, op);
                }
                else if (op.receipt.status === 'FAILED_NO_EFFECT') {
                    finish(o, op);
                }
                else {
                    const accepted = await readOperationAdmission(o, op, op.command.kind === 'create' ? op.receipt.table_id : undefined);
                    if (accepted)
                        settleFromAdmission(o, op, accepted);
                }
                return;
            }
            const result = await readPokerEntryReceipt(o.api, op.scope, () => current(o) ? o.scope : null, op.command);
            if (!result || !entryCurrent(o, op))
                return;
            if (result.state === 'FOUND') {
                await acceptReceipt(o, op, result.receipt);
                return;
            }
            o.notice = 'NOT_FOUND 仅代表当前不可见；系统会保留原请求键并自动继续只读核对。';
            show(o);
        }
        catch {
            if (entryCurrent(o, op))
                o.notice = '原记录暂未核实；系统会按退避节奏继续只读查询。';
        }
        finally {
            if (entryCurrent(o, op)) {
                o.busy = false;
                show(o);
                if (pending(o))
                    scheduleRecovery(o, op);
            }
        }
    }
    async function refreshLease(o: Owner) {
        const lease = o.lease;
        if (!current(o) || !lease || o.leaseReading || pending(o))
            return;
        o.leaseReading = lease;
        try {
            const result = await queryLease(o, lease.table, lease.id, lease.seat, lease);
            if (result && current(o))
                clearLeaseNotice(o);
        }
        catch {
            if (current(o) && o.lease === lease && !pending(o)) {
                o.lease = { ...lease, remaining: 0 };
                const notice = '预留状态暂未核实；系统将在网络恢复后自动核对。';
                if (!o.notice || o.notice === o.leaseNotice) {
                    o.notice = notice;
                    o.leaseNotice = notice;
                }
            }
        }
        finally {
            if (current(o) && o.leaseReading === lease) {
                delete o.leaseReading;
                show(o);
            }
        }
    }
    async function enter(o: Owner, value: Extract<LobbyIntent, {
        type: 'reconnect';
    }> | {
        type: 'spectate';
        table_id: string;
    }) {
        o.busy = true;
        show(o);
        try {
            const accepted = await targetRead(o, value.type === 'spectate' ? value.table_id : undefined);
            if (!accepted || !fresh(o) || pending(o) || o.entered)
                return;
            const active = accepted.snapshot.active_session, row = accepted.snapshot.tables.find(t => t.table_id === value.table_id);
            const allowed = value.type === 'reconnect' ? active?.table_id === value.table_id && active.session_id === value.session_id && active.can_reconnect :
                !active && row?.can_spectate===true;
            if (allowed) {
                o.entered = true;
                latest.current.onEnter(value.table_id, accepted);
            }
            else
                o.notice = '当前整包尚未确认原入桌目标，请刷新后核对。';
        }
        finally {
            if (current(o)) {
                o.busy = false;
                show(o);
            }
        }
    }
    function intent(o: Owner, value: LobbyIntent, context: PokerLobbyReadScope) {
        if (!current(o))
            return;
        if (value.type === 'refresh') {
            void read(o);
            return;
        }
        if (pending(o) || o.busy || o.entered || !fresh(o, context))
            return;
        if (value.type === 'wallet' || value.type === 'history' || value.type === 'rewards') {
            latest.current.onNavigate(value.type);
            return;
        }
        if (value.type === 'reconnect' || value.type === 'spectate') {
            const s = o.accepted!.snapshot, active = s.active_session, row = s.tables.find(t => t.table_id === value.table_id);
            if (value.type === 'reconnect' ? active?.table_id === value.table_id && active.session_id === value.session_id && active.can_reconnect :
                !active && row?.can_spectate===true)
                void enter(o, value.type === 'reconnect' ? value : { type: 'spectate', table_id: value.table_id });
            return;
        }
        if (value.type === 'open_table') {
            if (permitted(o) && o.accepted!.snapshot.tables.some(t => t.table_id === value.table_id && (t.can_join || t.visibility === 'PASSWORD' && t.can_request_access))) {
                o.scope = { ...o.scope, entry_generation: o.scope.entry_generation + 1 };
                o.operation = undefined;
                o.lease = undefined;
                join(o, value.table_id);
                show(o);
            }
            return;
        }
        if(value.type==='access'&&o.flow.kind==='JOIN'&&value.table_id===o.flow.table_id){
            const row=o.accepted!.snapshot.tables.find(t=>t.table_id===value.table_id),password=o.flow.draft.password;
            if(!policyReady()||!row||row.visibility!=='PASSWORD'||!row.can_request_access||value.password!==password||password.length===0||new TextEncoder().encode(password).length>128)return;
            const accessScope={...o.scope};o.busy=true;o.notice='';show(o);
            void(async()=>{
                try{
                    const granted=await verifyPokerTableAccess(o.api,accessScope,()=>current(o)?o.scope:null,value.table_id,password,policyReady);
                    if(!granted||!current(o)||o.flow.kind!=='JOIN'||o.flow.table_id!==value.table_id)return;
                    o.flow={...o.flow,access_granted:true,can_reserve:false,draft:{...o.flow.draft,password:''}};show(o);
                    const accepted=await targetRead(o,value.table_id),freshRow=accepted?.snapshot.tables.find(t=>t.table_id===value.table_id);
                    if(current(o)&&o.flow.kind==='JOIN'&&o.flow.table_id===value.table_id){
                        o.flow={...o.flow,access_granted:true,can_reserve:freshRow?.can_join===true,draft:{...o.flow.draft,password:''}};
                        o.notice=freshRow?.can_join||freshRow?.can_spectate?'密码已验证；请选择座位或进入观战。':'密码已验证；当前牌桌暂不接受新入口。';show(o);
                    }
                }catch(error){if(current(o)){o.notice=error instanceof Error?error.message:'牌桌访问权限尚未确认。';show(o);}}
                finally{if(current(o)){o.busy=false;show(o);}}
            })();return;
        }
        let command: PokerEntryCommand;
        if (value.type === 'create' && o.flow.kind === 'CREATE') {
            const draft = o.flow.draft;
            if (value.name !== draft.name.trim() || !value.name || /[\p{Cc}\p{Cs}]/u.test(value.name) || Array.from(new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(value.name)).length > 40 ||
                !Number.isInteger(value.max_seats) || value.max_seats < 2 || value.max_seats > 9 || value.max_seats !== draft.max_seats || value.blind_preset_id !== draft.blind_preset_id ||
                value.allow_spectators !== draft.allow_spectators || value.visibility!==draft.visibility || value.chat_enabled!==draft.chat_enabled ||
                !o.accepted!.snapshot.create_options.access_modes.includes(value.visibility)||value.chat_enabled&&!o.accepted!.snapshot.create_options.chat_configurable||
                (value.visibility==='PASSWORD'?(value.password!==draft.password||draft.password.length===0||new TextEncoder().encode(draft.password).length>128):value.password!==undefined))
                return;
            command = { kind: 'create', request_id: 'validation-placeholder', name: value.name, blind_preset: value.blind_preset_id, max_seats: value.max_seats, allow_spectators: value.allow_spectators, access_mode:value.visibility,...(value.visibility==='PASSWORD'?{password:value.password}:{}),chat_enabled:value.chat_enabled };
        }
        else if (value.type === 'reserve' && o.flow.kind === 'JOIN' && value.table_id === o.flow.table_id) {
            command = { kind: 'reserve', request_id: 'validation-placeholder', table_id: value.table_id, seat_no: value.seat_no };
        }
        else if (value.type === 'buyin' && o.flow.kind === 'JOIN' && value.table_id === o.flow.table_id && value.entry_mode === 'WAIT_FOR_BB') {
            const lease = o.lease;
            if (chipInput(o.flow.draft.buy_in_chips)?.toString() !== value.amount_units || o.flow.draft.seat_no !== value.seat_no ||
                !lease || lease.table !== value.table_id || lease.id !== value.reservation_id || lease.seat !== value.seat_no || lease.result.state !== 'FOUND' || !lease.result.reservation.valid || remaining(lease) <= 0)
                return;
            command = { kind: 'buyin', request_id: 'validation-placeholder', table_id: value.table_id, reservation_id: value.reservation_id, amount_units: value.amount_units };
        }
        else
            return;
        if (!permitted(o, command))
            return;
        command.request_id = crypto.randomUUID();
        const op: Operation = { command: structuredClone(command), scope: { ...o.scope }, phase: 'SENT', seat: value.type === 'buyin' ? value.seat_no : undefined, recoveryAttempt: 0 };
        o.operation = op;
        o.busy = true;
        o.notice = '';
        latest.current.onPendingChange(true);
        show(o);
        void send(o, op);
    }
    const o = owned?.key === key && owned.api === p.client && owned.alive ? owned : null;
    if (!o?.accepted || !o.read)
        return <section role="status">等待完整大厅状态。{o?.notice}<button onClick={() => { if (o)
            void read(o); }}>刷新大厅</button></section>;
    const renderedEntry = o.scope;
    const row = o.flow.kind === 'JOIN' ? o.accepted.snapshot.tables.find(t => o.flow.kind === 'JOIN' && t.table_id === o.flow.table_id) : undefined;
    const flow: LobbyFlow = o.flow.kind === 'JOIN' ? { ...o.flow, seats: Array.from({ length: row?.max_seats ?? 0 }, (_, index) => ({ seat_no: index + 1, available: !!row?.open_seat_numbers.includes(index + 1) })), can_buy_in: remaining(o.lease) > 0 } : o.flow;
    const details = o.operation || o.lease ? <>
      {o.operation && <section aria-label="原入桌操作"><p role="status">{o.operation.command.kind} · {o.operation.command.request_id} · {o.operation.receipt?.status ?? o.operation.phase}</p>
        {pending(o) && <p>自动只读核对中，不会重发原请求。</p>}
      </section>}
      {o.lease && <section aria-label="服务端预留观察"><p role="status">预留剩余约 {Math.ceil(remaining(o.lease) / 1000)} 秒；状态由系统后台自动核对。</p></section>}
    </> : undefined;
    return <PokerLobby snapshot={o.accepted.snapshot} authority={{ scope: o.read, read_state: o.readState }} entry_pending={pending(o)} entry_blocked={p.policy.stage !== 'READY' || p.policy.mutation_blocked || p.policy.recovery_only} filters={o.filters} flow={flow} notice={o.notice || undefined} details={details} onFiltersChange={next => { if (current(o)) {
        o.filters = { ...next };
        o.filtered = true;
        void read(o);
    } }} onFlowChange={kind => {
            if (!current(o) || o.scope !== renderedEntry || pending(o) || o.busy || kind === 'CREATE' && (!permitted(o) || !o.accepted!.snapshot.viewer.can_create || o.accepted!.snapshot.viewer.owned_open_table_id))
                return;
            o.scope = { ...o.scope, entry_generation: o.scope.entry_generation + 1 };
            o.operation = undefined;
            o.lease = undefined;
            clearLeaseNotice(o);
            o.flow = kind === 'NONE' ? { kind: 'NONE' } : { kind: 'CREATE', draft: { name: '', visibility: 'PUBLIC', password: '', max_seats: 6, blind_preset_id: o.accepted!.snapshot.blind_presets[0]?.id ?? '', allow_spectators: true, chat_enabled: false } };
            show(o);
        }} onCreateDraftChange={draft => { if (current(o) && o.scope === renderedEntry && !pending(o) && o.flow.kind === 'CREATE') {
        o.flow = { ...o.flow, draft: { ...draft } };
        show(o);
    } }} onJoinDraftChange={draft => { if (current(o) && o.scope === renderedEntry && !pending(o) && o.flow.kind === 'JOIN') {
        o.flow = { ...o.flow, draft: { ...draft } };
        show(o);
    } }} onIntent={(value, context) => { if (o.scope === renderedEntry)
        intent(o, value, context); }}/>;
}
