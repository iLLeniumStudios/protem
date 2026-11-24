import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { SegmentedControl } from './SegmentedControl';

export type ThemeMode = 'system' | 'light' | 'dark';

// Key and resolution rule are mirrored by the pre-paint script in index.html.
const STORAGE_KEY = 'protem-theme';
const DARK_QUERY = '(prefers-color-scheme: dark)';

interface ThemeState {
  mode: ThemeMode;
  resolved: 'light' | 'dark';
  setMode: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeState | null>(null);

function readMode(): ThemeMode {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    // Storage blocked (private mode, sandbox): fall back to the OS setting.
  }
  return 'system';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(readMode);
  const [systemDark, setSystemDark] = useState(() => window.matchMedia(DARK_QUERY).matches);

  useEffect(() => {
    const mql = window.matchMedia(DARK_QUERY);
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  const resolved = mode === 'system' ? (systemDark ? 'dark' : 'light') : mode;

  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark');
  }, [resolved]);

  const value = useMemo<ThemeState>(
    () => ({
      mode,
      resolved,
      setMode: (next) => {
        setModeState(next);
        try {
          if (next === 'system') localStorage.removeItem(STORAGE_KEY);
          else localStorage.setItem(STORAGE_KEY, next);
        } catch {
          // Storage blocked: the choice lasts for this session only.
        }
      },
    }),
    [mode, resolved],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}

const OPTIONS = [
  { value: 'system' as const, label: 'System' },
  { value: 'light' as const, label: 'Light' },
  { value: 'dark' as const, label: 'Dark' },
];

export function ThemeToggle({ className }: { className?: string }) {
  const { mode, setMode } = useTheme();
  return <SegmentedControl label="Theme" value={mode} onValueChange={setMode} options={OPTIONS} className={className} />;
}
