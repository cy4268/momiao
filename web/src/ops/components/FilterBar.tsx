import { useEffect, useState, type ReactNode } from 'react';

export function FilterBar({children}:{children:ReactNode}) {
 return <div className="ops-filter-bar">{children}</div>;
}

export function useDebouncedValue<T>(value:T, delay=300) {
 const [settled,setSettled]=useState(value);
 useEffect(()=>{const timer=window.setTimeout(()=>setSettled(value),delay);return()=>window.clearTimeout(timer)},[value,delay]);
 return settled;
}
