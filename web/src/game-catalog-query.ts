import { z } from 'zod';

export const catalogAvailability = ['ALL','PLAY','MAINTENANCE','TEMPORARILY_UNAVAILABLE','COMING_SOON','RETIRED'] as const;
const validUnicode = (value:string) => { try { encodeURIComponent(value); return true; } catch { return false; } };
export const catalogQuerySchema = z.strictObject({
    q: z.string().refine(validUnicode).refine(value=>!/[\u0000-\u001f\u007f-\u009f]/u.test(value))
        .transform(value=>value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu,''))
        .refine(value=>[...value].length<=128).default(''),
    availability: z.enum(catalogAvailability).default('ALL'),
    sort: z.enum(['RECOMMENDED','NAME']).default('RECOMMENDED'),
});
export type CatalogQuery = z.output<typeof catalogQuerySchema>;

export function parseCatalogQuery(search:string):CatalogQuery {
    const raw = search.startsWith('?') ? search.slice(1) : search;
    // Validate before URLSearchParams can replace bad UTF-8 or silently collapse malformed input.
    if (!validUnicode(raw) || raw.startsWith('?') || raw.includes(';')) throw new Error('目录筛选参数无效，请清除筛选后重试。');
    decodeURIComponent(raw.replace(/\+/g,' '));
    const values:Record<string,string> = {};
    for (const [key,value] of new URLSearchParams(raw)) {
        if (!['q','availability','sort'].includes(key) || Object.hasOwn(values,key)) throw new Error('目录筛选参数无效，请清除筛选后重试。');
        values[key] = value;
    }
    return catalogQuerySchema.parse(values);
}
export function serializeCatalogQuery(input:unknown):string {
    const query=catalogQuerySchema.parse(input), values=new URLSearchParams();
    if(query.q) values.set('q',query.q);
    if(query.availability!=='ALL') values.set('availability',query.availability);
    if(query.sort!=='RECOMMENDED') values.set('sort',query.sort);
    return values.size ? '?'+values.toString() : '';
}
const publicEntrySchema=z.object({
    slug:z.string().regex(/^[a-z][a-z0-9-]*$/), title:z.string().min(1),
    effective_runtime:z.enum(['PLAY','MAINTENANCE','TEMPORARILY_UNAVAILABLE','COMING_SOON','RETIRED']),
    implementation_key:z.string().min(1),
});
const publicCatalogSchema=z.object({items:z.array(publicEntrySchema)
    .refine(items=>new Set(items.map(item=>item.slug)).size===items.length)});
export function parsePublicCatalog(data:unknown) { return publicCatalogSchema.parse(data); }
