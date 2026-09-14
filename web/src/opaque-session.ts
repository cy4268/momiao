export type OpaqueAuthenticationState =
    'ANONYMOUS' | 'AUTHENTICATION_PENDING' | 'TWO_FA' | 'DISCORD_CALLBACK' | 'DISCORD_TWO_FA' | 'CANCELLED' | 'UNKNOWN' | 'AUTHENTICATED';

export interface OpaquePasswordProvider {
    state: 'AVAILABLE' | 'DISABLED' | 'UNAVAILABLE';
    turnstileRequired: boolean;
    turnstileSiteKey?: string;
}

export interface OpaqueBootstrap {
    authenticationState: OpaqueAuthenticationState;
    csrfToken: string;
    expiresAt: number;
    identityID?: string;
    authChainStartedAt?: string;
    authChainID?: string;
    password: OpaquePasswordProvider;
    unavailable: string[];
}

export type OpaqueLoginResult = {
    authenticationState: 'AUTHENTICATED';
    csrfToken: string;
} | {
    authenticationState: 'TWO_FA';
    csrfToken: string;
    challengeID: string;
    expiresAt: string;
    expiresAtSeconds: number;
};

export interface OpaqueLogoutResult {
    platformSession: string;
    nativeSession: string;
    pokerControl: string;
}

const token = (value: unknown): value is string => typeof value === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value);
const object = (value: unknown): Record<string, unknown> => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('opaque protocol object');
    return value as Record<string, unknown>;
};
const time = (value: unknown): number => {
    if (typeof value !== 'string') throw new Error('opaque protocol time');
    const parsed = Date.parse(value) / 1000;
    if (!Number.isFinite(parsed)) throw new Error('opaque protocol time');
    return parsed;
};
const csrf = (value: unknown): string => {
    if (!token(value)) throw new Error('opaque protocol csrf');
    return value;
};

function password(value: unknown): OpaquePasswordProvider {
    const input = object(value);
    if (!['AVAILABLE', 'DISABLED', 'UNAVAILABLE'].includes(String(input.state)) || typeof input.turnstile_required !== 'boolean') throw new Error('opaque password capability');
    if (input.turnstile_site_key !== undefined && (typeof input.turnstile_site_key !== 'string' || !input.turnstile_site_key)) throw new Error('opaque password capability');
    if (input.turnstile_required && typeof input.turnstile_site_key !== 'string') throw new Error('opaque password capability');
    return {
        state: input.state as OpaquePasswordProvider['state'],
        turnstileRequired: input.turnstile_required,
        turnstileSiteKey: input.turnstile_site_key as string | undefined,
    };
}

function unavailable(value: unknown): string[] {
    if (!Array.isArray(value) || value.some(item => typeof item !== 'string' || !item)) throw new Error('opaque capabilities');
    return [...new Set(value)];
}

export function parseOpaqueBootstrap(value: unknown): OpaqueBootstrap {
    const input = object(value);
    const state = String(input.authentication_state) as OpaqueAuthenticationState;
    if (!['ANONYMOUS', 'AUTHENTICATION_PENDING', 'TWO_FA', 'DISCORD_CALLBACK', 'DISCORD_TWO_FA', 'CANCELLED', 'UNKNOWN', 'AUTHENTICATED'].includes(state)) throw new Error('opaque authentication state');
    const base = { authenticationState: state, csrfToken: csrf(input.csrf_token), password: password(input.password), unavailable: unavailable(input.unavailable) };
    if (state !== 'AUTHENTICATED') return { ...base, expiresAt: time(input.anonymous_expires_at) };
    const identity = object(input.identity);
    if (typeof identity.newapi_user_id !== 'string' || !/^[1-9]\d*$/.test(identity.newapi_user_id)) throw new Error('opaque identity');
    const session = object(input.session);
    const absolute = time(session.absolute_expires_at), idle = time(session.idle_expires_at);
    time(session.auth_chain_started_at);
    if (!token(session.auth_chain_id)) throw new Error('opaque auth chain');
    const authChainID = session.auth_chain_id;
    if (session.fresh_auth_at !== null && session.fresh_auth_at !== undefined) time(session.fresh_auth_at);
    if (typeof session.fresh_auth_method !== 'string') throw new Error('opaque session');
    return { ...base, expiresAt: Math.min(absolute, idle), identityID: identity.newapi_user_id, authChainStartedAt: session.auth_chain_started_at as string, authChainID };
}

export function parseOpaqueLogin(value: unknown): OpaqueLoginResult {
    const input = object(value), state = input.authentication_state, csrfToken = csrf(input.csrf_token);
    if (state === 'AUTHENTICATED') return { authenticationState: state, csrfToken };
    if (state !== 'TWO_FA') throw new Error('opaque login state');
    const challenge = object(input.challenge);
    if (!token(challenge.challenge_id) || typeof challenge.expires_at !== 'string') throw new Error('opaque challenge');
    return { authenticationState: state, csrfToken, challengeID: challenge.challenge_id, expiresAt: challenge.expires_at, expiresAtSeconds: time(challenge.expires_at) };
}

export function parseOpaqueLogout(value: unknown): OpaqueLogoutResult {
    const input = object(value);
    const states = ['REVOKED', 'NOT_ISSUED', 'PENDING_COMPENSATION', 'UNKNOWN', 'UNAVAILABLE_S2'];
    for (const key of ['platform_session', 'native_session', 'poker_control']) if (typeof input[key] !== 'string' || !states.includes(input[key])) throw new Error('opaque logout result');
    return { platformSession: input.platform_session as string, nativeSession: input.native_session as string, pokerControl: input.poker_control as string };
}

export type OpaqueAdmissionResult =
    { authenticationState: 'AUTHENTICATED'; csrfToken: string } |
    { authenticationState: 'DISCORD_TWO_FA'; csrfToken: string; challengeID: string; expiresAt: string; expiresAtSeconds: number } |
    { authenticationState: 'ACCOUNT_PROOF_READY'; csrfToken: string; purpose: 'PASSWORD_SET' | 'PASSWORD_RESET'; expiresAtSeconds: number };

export function parseOpaqueDiscordStart(value: unknown) {
    const input = object(value);
    if (input.authentication_state !== 'DISCORD_CALLBACK' || typeof input.authorization_url !== 'string') throw new Error('opaque Discord start');
    return { csrfToken: csrf(input.csrf_token), authorizationURL: input.authorization_url };
}

export function parseOpaqueAdmission(value: unknown): OpaqueAdmissionResult {
    const input = object(value), state = input.authentication_state, csrfToken = csrf(input.csrf_token);
    if (state === 'AUTHENTICATED') return { authenticationState: state, csrfToken };
    if (state === 'ACCOUNT_PROOF_READY') {
        if (input.proof_purpose !== 'PASSWORD_SET' && input.proof_purpose !== 'PASSWORD_RESET') throw new Error('opaque proof purpose');
        return { authenticationState: state, csrfToken, purpose: input.proof_purpose, expiresAtSeconds: time(input.expires_at) };
    }
    if (state !== 'DISCORD_TWO_FA') throw new Error('opaque Discord result');
    const challenge = object(input.challenge);
    if (!token(challenge.challenge_id) || typeof challenge.expires_at !== 'string') throw new Error('opaque Discord challenge');
    return { authenticationState: state, csrfToken, challengeID: challenge.challenge_id, expiresAt: challenge.expires_at, expiresAtSeconds: time(challenge.expires_at) };
}

export const opaqueLogoutConfirmed = (result: OpaqueLogoutResult) =>
    result.platformSession === 'NOT_ISSUED' && ['NOT_ISSUED','REVOKED'].includes(result.nativeSession) && result.pokerControl === 'UNAVAILABLE_S2' ||
    result.platformSession === 'REVOKED' && result.nativeSession === 'REVOKED' && ['REVOKED', 'UNAVAILABLE_S2'].includes(result.pokerControl);
