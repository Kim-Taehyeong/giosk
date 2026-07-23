import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './styles/console.css'
import './i18n';
import App from './App.jsx'
import { AuthProvider } from './context/AuthContext';
import { ThemeProvider } from './context/ThemeContext';
import { ConsoleProvider } from './context/ConsoleContext';
import { SystemConfigProvider } from './context/SystemConfigContext';
import { ToastProvider } from './components/console/Toast';
import { ConfirmProvider } from './components/console/Confirm';

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <ThemeProvider>
      <AuthProvider>
        <SystemConfigProvider>
          <ConsoleProvider>
            <ToastProvider>
              <ConfirmProvider>
                <App />
              </ConfirmProvider>
            </ToastProvider>
          </ConsoleProvider>
        </SystemConfigProvider>
      </AuthProvider>
    </ThemeProvider>
  </StrictMode>,
)
