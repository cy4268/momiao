import manifest from './game-hall-assets.json';

export type ImageAsset = {
    src: string;
    width: number;
    height: number;
    variants: { src: string; width: number }[];
};

export const hallArt = manifest;
const covers = new Map<string, ImageAsset>(Object.entries(manifest.games));

export function gameArt(slug: string): ImageAsset | undefined {
    return covers.get(slug);
}

export function assetUrl(key: string): string {
    const base = (import.meta.env.VITE_ASSET_BASE_URL ?? '').replace(/\/+$/, '');
    return base + '/' + key.replace(/^\/+/, '');
}

export function assetSrcSet(asset: ImageAsset): string {
    return asset.variants.map(item => assetUrl(item.src) + ' ' + item.width + 'w').join(', ');
}
