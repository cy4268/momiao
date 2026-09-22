import {assetUrl} from '../game-hall-assets';
import art from './roulette-art.json';
export const rouletteArt=art;
export const rouletteImage=(name:keyof typeof art)=>assetUrl(art[name].src);

const items = {magnifier:0, phone:1, handcuffs:2, adrenaline:3, saw:4, inverter:5, beer:6, cigarette:7, medicine:8};
const ornaments = {compass:0, lyre:1, lily:2, eagle:3, moon:4, crown:5, life:6, spentLife:7, live:8, blank:9, lock:10, hourglass:11, self:12, opponent:13, cylinder:14, ammoBox:15};
export const seatCrests = ['compass','lyre','lily','eagle','moon','crown'] as const;

/** Decorative raster artwork; adjacent live text always carries the game state. */
export function RouletteArt({name,className=''}:{name:keyof typeof items|keyof typeof ornaments;className?:string}) {
 const item=name in items, size=item?3:4;
 const index=item?items[name as keyof typeof items]:ornaments[name as keyof typeof ornaments];
 // The generated ornament rows have uneven gutters. Sample their actual 300px
 // cells, not a guessed equal grid, so adjacent artwork never bleeds through.
 const x=item?index%size/(size-1)*100:[21,326,632,942][index%4]/954*100;
 const y=item?Math.floor(index/size)/(size-1)*100:[33,321,614,898][Math.floor(index/4)]/954*100;
 const scale=item?300:1254/300*100;
 return <span aria-hidden="true" data-art={name} className={'roulette-art '+className} style={{
  backgroundImage:`url(${rouletteImage(item?'items':'ornaments')})`,
  backgroundSize:`${scale}% ${scale}%`,
  backgroundPosition:`${x}% ${y}%`,
 }}/>;
}
