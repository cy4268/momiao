import { expect, it } from 'vitest';
import { parseProfile, profileError } from './profile-api';
import { ApiError } from './api';
const incomplete = { user_id: '1', short_account_id: 'CA-123456789ABC', status: 'INCOMPLETE', display_name: '', avatar_id: 'system-default', profile_version: '0', nickname_changed_at: null, next_rename_at: null, suggested_name: 'Master-CA-123456789ABC', avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM' }] };
const complete = { ...incomplete, status: 'COMPLETE', display_name: 'Moonlit', profile_version: '9007199254740993' };
it('preserves canonical int64 IDs and versions above Number precision', () => {
    expect(parseProfile({ ...complete, user_id: '9223372036854775807' }, '9223372036854775807')).toMatchObject({ user_id: '9223372036854775807', profile_version: '9007199254740993' });
    expect(parseProfile(incomplete, '1').status).toBe('INCOMPLETE');
});
it.each([0, 1, null, undefined, '', '01', '-1', '1.0', '9223372036854775808'])('rejects malformed profile version %s', version => {
    expect(() => parseProfile({ ...complete, profile_version: version }, '1')).toThrow(/资料响应格式异常/);
});
it('requires complete and incomplete fields to agree instead of inventing defaults', () => {
    for (const patch of [{ status: 'OTHER' }, { status: 'INCOMPLETE' }, { display_name: '' }, { display_name: null }, { profile_version: '0' }, { avatar_id: null }, { nickname_changed_at: undefined }, { next_rename_at: undefined }, { password: 'unexpected' }]) expect(() => parseProfile({ ...complete, ...patch }, '1')).toThrow();
    for (const patch of [{ display_name: 'Auto' }, { profile_version: '1' }, { nickname_changed_at: '2026-09-05T00:00:00Z', next_rename_at: '2026-09-12T00:00:00Z' }]) expect(() => parseProfile({ ...incomplete, ...patch }, '1')).toThrow();
});
it('rejects foreign users, malformed short IDs and mismatched private suggestions', () => {
    for (const patch of [{ user_id: '2' }, { user_id: 1 }, { user_id: '0' }, { user_id: '01' }, { short_account_id: 'CA-abc' }, { short_account_id: 'CA-123456789abc' }, { suggested_name: 'Admin' }, { suggested_name: null }]) expect(() => parseProfile({ ...complete, ...patch }, '1')).toThrow();
});
it('validates a closed system avatar catalog and never accepts remote URLs', () => {
    for (const patch of [{ avatar_id: 'https://example.invalid/avatar' }, { avatars: [] }, { avatars: [incomplete.avatars[0], incomplete.avatars[0]] }, { avatars: [{ id: 'system-default', label: '系统默认头像', source: 'UPLOAD' }] }, { avatars: [{ id: 'system-default', label: '系统默认头像', source: 'SYSTEM', url: 'https://example.invalid' }] }]) expect(() => parseProfile({ ...complete, ...patch }, '1')).toThrow();
});
it('accepts UTC RFC3339 with subseconds and rejects invalid, unmatched or reversed rename times', () => {
    expect(parseProfile({ ...complete, nickname_changed_at: '2026-09-05T01:02:03.123456Z', next_rename_at: '2026-09-12T01:02:03.123456Z' }, '1').next_rename_at).toBe('2026-09-12T01:02:03.123456Z');
    for (const t of ['bad', '2026-09-05', '2026-02-30T01:00:00Z', '2026-09-05T01:00:00+08:00', 0, undefined]) expect(() => parseProfile({ ...complete, nickname_changed_at: t, next_rename_at: '2026-09-12T01:00:00Z' }, '1')).toThrow();
    expect(() => parseProfile({ ...complete, nickname_changed_at: null, next_rename_at: '2026-09-12T01:00:00Z' }, '1')).toThrow();
    expect(() => parseProfile({ ...complete, nickname_changed_at: '2026-09-12T01:00:00Z', next_rename_at: '2026-09-05T01:00:00Z' }, '1')).toThrow();
});
it('unknown or prototype-like error codes produce a plain generic message without server detail', () => {
    for (const code of ['UNKNOWN', 'constructor', '__proto__', 'toString']) {
        const message = profileError(new ApiError('private server detail', 409, code));
        expect(typeof message).toBe('string'); expect(message).not.toContain('private server detail');
    }
});
