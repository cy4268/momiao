import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import '@fontsource/marcellus/latin-400.css';
import { App } from './App';
import './styles.css';
import './entry.css';
import './admission.css';
import { captureDiscordCallback } from './admission-api';
import type { CapturedCallback } from './Authentication';
// Capture and scrub before React mounts or any bootstrap effect can request data.
let capturedCallback:CapturedCallback|undefined;
if(window.location.pathname==='/oauth/discord'){
    try{capturedCallback={input:captureDiscordCallback(window.location,window.history)};}
    catch(e){capturedCallback={error:e instanceof Error?e.message:'授权回调已失效，请重新开始。'};}
}
createRoot(document.getElementById('root')!).render(<BrowserRouter><App capturedCallback={capturedCallback}/></BrowserRouter>);
