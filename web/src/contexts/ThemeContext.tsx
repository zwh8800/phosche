/**
 * 主题上下文
 *
 * 提供全局主题状态管理，支持 3 套主题切换：
 * - default：默认浅色主题（红色 accent）
 * - apple：Apple 设计风格（蓝色 accent，SF Pro 字体）
 * - voltagent：Voltagent 深色主题（绿色 accent，Inter 字体）
 *
 * 主题选择持久化到 localStorage（key: 'phosche-theme'），
 * 切换时同步更新 document.documentElement.dataset.theme 属性，
 * 触发 CSS 变量重新计算，实现即时主题切换。
 */
import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';

/** 可用主题列表 */
export const THEMES = ['default', 'apple', 'voltagent'] as const;

/** 主题类型 */
export type Theme = (typeof THEMES)[number];

/** 主题上下文值类型 */
interface ThemeContextValue {
  /** 当前主题 */
  theme: Theme;
  /** 设置主题 */
  setTheme: (theme: Theme) => void;
  /** 可用主题列表 */
  themes: readonly Theme[];
}

/** localStorage 存储键 */
const STORAGE_KEY = 'phosche-theme';

/** 主题上下文 */
const ThemeContext = createContext<ThemeContextValue | null>(null);

/**
 * 从 localStorage 读取初始主题
 * 如果存储值无效或不存在，返回 'default'
 */
function getInitialTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && THEMES.includes(stored as Theme)) {
      return stored as Theme;
    }
  } catch {
    // localStorage 不可用时静默忽略
  }
  return 'default';
}

/**
 * 主题提供者组件
 *
 * 管理主题状态，同步到 DOM 和 localStorage
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(getInitialTheme);

  /**
   * 设置主题
   * 同步更新 state、localStorage 和 DOM 属性
   */
  const setTheme = (t: Theme) => {
    setThemeState(t);
    try {
      localStorage.setItem(STORAGE_KEY, t);
    } catch {
      // localStorage 不可用时静默忽略
    }
    document.documentElement.dataset.theme = t;
    document.documentElement.style.colorScheme = t === 'voltagent' ? 'dark' : 'light';
  };

  /** 组件挂载时同步 DOM 属性 */
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme === 'voltagent' ? 'dark' : 'light';
  }, [theme]);

  return (
    <ThemeContext.Provider value={{ theme, setTheme, themes: THEMES }}>
      {children}
    </ThemeContext.Provider>
  );
}

/**
 * 使用主题的 Hook
 *
 * @returns 主题上下文值，包含 theme、setTheme、themes
 * @throws 如果在 ThemeProvider 外使用则抛出错误
 */
export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) {
    throw new Error('useTheme must be used within ThemeProvider');
  }
  return ctx;
}
