import { useState, type ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { CommandChamber } from './PersonalHub';
import { Modal } from './ui';
import { assetUrl } from './game-hall-assets';
import art from './command-personal-art.json';
import './identity-chamber.css';

export function IdentityChamber({ page, children }: { page: 'profile' | 'security'; children: ReactNode }) {
    return <CommandChamber page={page} className="identity-chamber">
        <nav className="identity-navigation" aria-label="个人页面导航"><NavLink to="/me">个人中心</NavLink><NavLink to="/master-profile">Master 资料</NavLink><NavLink to="/account">账户与安全</NavLink></nav>
        {children}
    </CommandChamber>;
}

export function IdentityEmblem() {
    return <img className="identity-emblem" src={assetUrl(art.emblem.src)} width={art.emblem.width} height={art.emblem.height} alt="" />;
}

export function IdentityDisclosure({ title, children }: { title: string; children: ReactNode }) {
    const [open, setOpen] = useState(false);
    return <><button className="identity-disclosure" type="button" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen(true)}>{title}<span aria-hidden="true">＋</span></button>
        {open && <Modal title={title} onClose={() => setOpen(false)}><div className="identity-explanation">{children}</div></Modal>}
    </>;
}
