import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import '@fontsource/marcellus/latin-400.css';
import { App } from './App';
import './styles.css';
createRoot(document.getElementById('root')!).render(<BrowserRouter><App /></BrowserRouter>);
