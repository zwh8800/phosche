import { useTheme, type Theme } from '../contexts/ThemeContext';

const THEME_LABELS: Record<Theme, string> = {
  default: '默认',
  apple: 'Apple',
  voltagent: 'Voltagent',
};

export default function ThemeSwitcher() {
  const { theme, setTheme } = useTheme();

  return (
    <select
      value={theme}
      onChange={(e) => setTheme(e.target.value as Theme)}
      aria-label="切换主题"
      className="rounded-theme-md border border-border-default bg-surface-card px-2 py-1 text-xs text-text-secondary cursor-pointer focus:outline-none focus:ring-2 focus:ring-accent-ring"
    >
      <option value="default">{THEME_LABELS.default}</option>
      <option value="apple">{THEME_LABELS.apple}</option>
      <option value="voltagent">{THEME_LABELS.voltagent}</option>
    </select>
  );
}
