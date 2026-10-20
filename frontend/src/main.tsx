import React from 'react'
import {createRoot} from 'react-dom/client'
// Bundled font: only the Unicode ranges a screen uses are loaded, and no
// network is needed.
import 'pretendard/dist/web/variable/pretendardvariable-dynamic-subset.css'
import './style.css'
import App from './App'
import {applyTheme} from './ui'

// Follow the OS until the saved setting is read, so the first paint is
// already in the right theme.
applyTheme('system')

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
)
