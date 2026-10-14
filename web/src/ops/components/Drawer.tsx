import type { ReactNode } from 'react';
import { Modal } from '../../ui';

export function Drawer({title,children,footer,onClose,busy=false}:{title:string;children:ReactNode;footer?:ReactNode;onClose:()=>void;busy?:boolean}) {
 return <Modal title={title} onClose={onClose} busy={busy} className="ops-drawer"><div className="ops-drawer-body">{children}</div>{footer&&<div className="ops-drawer-footer">{footer}</div>}</Modal>;
}
