/**
 * 应用主布局组件
 *
 * 提供整个 SPA 的页面骨架结构，包括：
 * - 顶部导航栏（左侧品牌标识 + 右侧导航链接）
 * - 响应式设计：桌面端显示水平导航链接，移动端显示汉堡菜单按钮
 * - 移动端展开的下拉菜单面板
 * - 主内容区域（通过 React Router 的 Outlet 渲染当前路由的子页面）
 *
 * 导航链接包含"时间线"和"搜索"两个入口，
 * 使用 NavLink 实现当前活动页面的高亮样式。
 */
import { useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';

/**
 * Layout 组件 — 页面布局容器
 *
 * 使用 flex 列布局确保内容区域撑满视口高度。
 * 响应式断点使用 Tailwind 的 md（768px）：
 * - >= md：水平导航栏 + 内联导航链接
 * - < md：水平导航栏 + 汉堡菜单按钮 + 可展开的垂直菜单
 */
function Layout() {
  /** 移动端菜单展开/收起状态，默认收起 */
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  /** 导航链接的基础 CSS 类名：内边距、圆角、字号、过渡动画 */
  const baseLinkClass = 'px-3 py-2 rounded text-sm font-medium transition-colors';
  /** 当前活动链接的高亮样式：白色文字 + 红色背景 */
  const activeClass = 'text-white bg-red-600';
  /** 非活动链接的默认样式：灰色文字 + 悬停效果 */
  const inactiveClass = 'text-gray-700 hover:text-gray-900 hover:bg-gray-100';

  /**
   * 导航链接列表
   * 在桌面端和移动端菜单中复用，点击任意链接时关闭移动端菜单。
   * NavLink 的 isActive 参数自动判断当前路由，动态切换 active/inactive 样式。
   */
  const navLinks = (
    <>
      <NavLink
        to="/"
        end
        onClick={() => setMobileMenuOpen(false)}
        className={({ isActive }) =>
          `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
        }
      >
        时间线
      </NavLink>
      <NavLink
        to="/search"
        onClick={() => setMobileMenuOpen(false)}
        className={({ isActive }) =>
          `${baseLinkClass} ${isActive ? activeClass : inactiveClass}`
        }
      >
        搜索
      </NavLink>
    </>
  );

  return (
    <div className="min-h-screen bg-gray-50">
      {/*
        顶部导航栏
        - 白色背景 + 底部浅灰色分隔线
        - 内容区域最大宽度 6xl，居中布局
        - 水平排列：品牌标识（左） + 导航链接（右）
       */}
      <nav className="bg-white shadow-sm border-b border-gray-200">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-14">
            {/*
              品牌标识：点击返回首页
              使用粗体文字而非图片，减少加载依赖
            */}
            <NavLink to="/" className="text-lg font-bold text-gray-900 tracking-tight">
              Phosche
            </NavLink>

            {/*
              桌面端导航链接（md 及以上屏幕显示）
              使用 flex 行布局，水平排列链接项
            */}
            <div className="hidden md:flex items-center gap-1">
              {navLinks}
            </div>

            {/*
              移动端汉堡菜单按钮（md 以下屏幕显示）
              根据 mobileMenuOpen 状态切换图标：
              - 菜单关闭时：显示三条横线（汉堡图标）
              - 菜单打开时：显示 X 形状（关闭图标）
            */}
            <button
              type="button"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="md:hidden inline-flex items-center justify-center p-2 rounded-md text-gray-500 hover:text-gray-700 hover:bg-gray-100 transition-colors cursor-pointer"
              aria-label="切换菜单"
            >
              {mobileMenuOpen ? (
                /* 关闭图标：X 形状 */
                <svg
                  className="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              ) : (
                /* 汉堡图标：三条横线 */
                <svg
                  className="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M4 6h16M4 12h16M4 18h16"
                  />
                </svg>
              )}
            </button>
          </div>
        </div>

        {/*
          移动端展开的下拉菜单
          仅在 mobileMenuOpen 为 true 时渲染
          垂直排列所有导航链接，覆盖在导航栏下方
        */}
        {mobileMenuOpen && (
          <div className="md:hidden border-t border-gray-200 bg-white">
            <div className="max-w-6xl mx-auto px-4 py-2 flex flex-col gap-1">
              {navLinks}
            </div>
          </div>
        )}
      </nav>
      {/*
        主内容区域
        使用 Outlet 渲染当前路由对应的子页面组件
        与导航栏保持相同的水平内边距和最大宽度
      */}
      <main className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <Outlet />
      </main>
    </div>
  );
}

export default Layout;
