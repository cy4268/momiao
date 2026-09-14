import { expect, it } from 'vitest';
import { catalogQuerySchema, parseCatalogQuery, serializeCatalogQuery, parsePublicCatalog } from './game-catalog-query';
import { canEnterCatalogGame } from './Games';

it.each([
    ['', '', 'ALL', 'RECOMMENDED'],
    ['?q=+Dice+&availability=PLAY&sort=NAME', 'Dice', 'PLAY', 'NAME'],
    ['?q=%E3%80%80%E9%AA%B0%E5%AD%90%C2%A0', '骰子', 'ALL', 'RECOMMENDED'],
    ['?q='+encodeURIComponent('🎲'.repeat(128)), '🎲'.repeat(128), 'ALL', 'RECOMMENDED'],
    ['?q=%EF%BB%BFDice&availability=ALL&sort=RECOMMENDED', '\ufeffDice', 'ALL', 'RECOMMENDED'],
    ['?q=a%3Bb%26c%2Bd', 'a;b&c+d', 'ALL', 'RECOMMENDED'],
])('validates and round trips URL %s', (raw,q,availability,sort)=>{
    const expected={q,availability,sort};
    expect(parseCatalogQuery(raw)).toEqual(expected);
    expect(parseCatalogQuery(serializeCatalogQuery(expected))).toEqual(expected);
});

it.each(['?q=Dice','q=%','q=%G1','q=%FF','q=%C0%AF','q=%ED%A0%80','q=%F4%90%80%80','q=x&q=y','q=x&%71=y','q=x;sort=NAME','q=%00','q=%09Dice','q=%C2%85','q='+encodeURIComponent('🎲'.repeat(129)),'availability=','availability=RESUME','sort=','sort=POPULAR','sort=NAME&sort=NAME','user_id=101','Q=dice','q=\ud800'])('rejects malformed or unsupported URL %s',raw=>{
    expect(()=>parseCatalogQuery('?'+raw)).toThrow();
});

it('uses strict Zod input validation and a canonical public URL',()=>{
    expect(catalogQuerySchema.safeParse({q:'🎲'.repeat(129)}).success).toBe(false);
    expect(catalogQuerySchema.safeParse({q:'\ud800'}).success).toBe(false);
    expect(catalogQuerySchema.safeParse({q:'dice',owner:101}).success).toBe(false);
    expect(serializeCatalogQuery({q:' Dice ',availability:'PLAY',sort:'NAME'})).toBe('?q=Dice&availability=PLAY&sort=NAME');
    expect(serializeCatalogQuery({q:'',availability:'ALL',sort:'RECOMMENDED'})).toBe('');
});

it('validates real catalog data without imposing a game-count or implementation cap',()=>{
    const items=Array.from({length:12},(_,i)=>({slug:'future-'+i,title:'沙龙 '+i,effective_runtime:'COMING_SOON',implementation_key:'future.card.v1'}));
    expect(parsePublicCatalog({items}).items).toHaveLength(12);
    expect(()=>parsePublicCatalog({items:[{...items[0],slug:'../wallet'}]})).toThrow();
    expect(()=>parsePublicCatalog({items:[{...items[0],effective_runtime:'RESUME'}]})).toThrow();
    expect(()=>parsePublicCatalog({items:[items[0],items[0]]})).toThrow();
    expect(canEnterCatalogGame({...items[0],slug:'scratch',effective_runtime:'PLAY',implementation_key:'uploaded.script'})).toBe(false);
    expect(canEnterCatalogGame({...items[0],slug:'dice',effective_runtime:'PLAY',implementation_key:'direct.dice.v1'})).toBe(true);
    expect(canEnterCatalogGame({...items[0],slug:'blackjack',effective_runtime:'MAINTENANCE',implementation_key:'direct.blackjack.v1'})).toBe(true);
    expect(canEnterCatalogGame({...items[0],slug:'dice',effective_runtime:'MAINTENANCE',implementation_key:'direct.dice.v1'})).toBe(false);
});
