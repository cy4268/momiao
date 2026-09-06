import { beforeEach, expect, it } from 'vitest';
import { consumeRouteIntent, saveRouteIntent, peekRouteIntent, normalizeRouteIntent } from './post-auth-intent';

it('peeks without consuming until the route gate has verified the same destination',()=>{
 saveRouteIntent('/ops/announcements',1000);
 expect(peekRouteIntent(1001)).toBe('/ops/announcements');
 expect(peekRouteIntent(1002)).toBe('/ops/announcements');
 expect(consumeRouteIntent(1003)).toBe('/ops/announcements');
 expect(peekRouteIntent(1004)).toBe('/dashboard');
 expect(normalizeRouteIntent('/keys?command=delete')).toBeUndefined();
});

beforeEach(() => sessionStorage.clear());

it('retains only bounded model navigation across the existing provider round trip',()=>{
 for(const route of ['/api/access','/api/access?model_id=%E7%BB%84%2Fmodel&intent=use','/keys?model_id=a%3Fb%23c','/keys?model_id=a%3Bb','/ops/models']){
  saveRouteIntent(route,1000);expect(peekRouteIntent(1001)).toBe(route);expect(consumeRouteIntent(1002)).toBe(route);
 }
 for(const route of ['/api/access?intent=use','/api/access?model_id=x&intent=delete','/api/access?model_id=x&intent=use&intent=use','/keys?model_id=x&intent=use','/keys?model_id=x&model_id=y','/api/access?model_id=x&token=secret','/api/access?model_id=%FF','/api/access?model_id=%GG','/api/access?model_id=%20x','/keys?model_id=x#fragment','/api/access?']){
  expect(normalizeRouteIntent(route)).toBeUndefined();saveRouteIntent(route,1000);expect(peekRouteIntent(1001)).toBe('/dashboard');
 }
 expect(normalizeRouteIntent('/api/access?model_id=a;b')).toBeUndefined();
});

it('persists a stable permitted route, expires it and consumes it once', () => {
    saveRouteIntent('/wallet/activate', 1000);
    expect(JSON.parse(sessionStorage.getItem('chaldea.post-auth.route.v2')!)).toEqual({ route: '/wallet/activate', expires: 1801000 });
    expect(consumeRouteIntent(1001)).toBe('/wallet/activate');
    expect(consumeRouteIntent(1002)).toBe('/dashboard');
    saveRouteIntent('/account', 1000);
    expect(consumeRouteIntent(1801000)).toBe('/dashboard');
});

it('removes the obsolete index-based format without interpreting it', () => {
    sessionStorage.setItem('chaldea.post-auth.route.v1', JSON.stringify({ route: 2, expires: 1801000 }));
    expect(consumeRouteIntent(1001)).toBe('/dashboard');
    expect(sessionStorage.getItem('chaldea.post-auth.route.v1')).toBeNull();
    sessionStorage.setItem('chaldea.post-auth.route.v1', 'stale');
    saveRouteIntent('/wallet', 1000);
    expect(sessionStorage.getItem('chaldea.post-auth.route.v1')).toBeNull();
});

it('rejects arbitrary URLs, unknown routes and extra or malformed data', () => {
    for (const route of ['https://evil.example', '//evil.example', '/wallet?amount=1000', '/unknown']) {
        saveRouteIntent(route, 1000);
        expect(consumeRouteIntent(1001)).toBe('/dashboard');
    }
    for (const data of [{ route: 2, expires: 1801000 }, { route: '/wallet', expires: 1801000, proof: 'synthetic' }, { route: '/wallet', expires: 9999999999 }]) {
        sessionStorage.setItem('chaldea.post-auth.route.v2', JSON.stringify(data));
        expect(consumeRouteIntent(1001)).toBe('/dashboard');
    }
});
