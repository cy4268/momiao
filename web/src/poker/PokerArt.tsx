import { assetSrcSet, assetUrl } from '../game-hall-assets';
import art from './poker-art.json';

/** Decorative art only: all seats, cards, amounts and controls remain real UI. */
export function PokerArt({name,className,sizes='100vw'}:{name:keyof typeof art;className?:string;sizes?:string}) {
  const asset=art[name];
  return <img className={className} src={assetUrl(asset.src)} srcSet={assetSrcSet(asset)} sizes={sizes}
    width={asset.width} height={asset.height} alt="" draggable={false} decoding="async"/>;
}
