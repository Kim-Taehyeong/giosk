import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';

const ThemeContext = createContext(null);

const STORAGE_KEY = 'giosk_theme';

// 문서 루트에 data-theme 속성(CSS 변수 구동)과 dark 클래스(Tailwind dark:)를 동시에 적용.
function applyTheme(theme) {
  const root = document.documentElement;
  if (theme === 'dark') {
    root.dataset.theme = 'dark';
    root.classList.add('dark');
  } else {
    delete root.dataset.theme;
    root.classList.remove('dark');
  }
}

export const ThemeProvider = ({ children }) => {
  const [theme, setThemeState] = useState(() => localStorage.getItem(STORAGE_KEY) || 'light');

  useEffect(() => {
    applyTheme(theme);
    localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  const setTheme = useCallback((t) => setThemeState(t === 'dark' ? 'dark' : 'light'), []);
  const toggle = useCallback(() => setThemeState((t) => (t === 'dark' ? 'light' : 'dark')), []);

  return (
    <ThemeContext.Provider value={{ theme, setTheme, toggle }}>
      {children}
    </ThemeContext.Provider>
  );
};

export const useTheme = () => useContext(ThemeContext);
