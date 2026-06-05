/**
 * 404 页面（未找到）
 *
 * 当用户访问不存在的路由时显示的兜底页面。
 * 这是 react-router 路由表中最后一条 <Route> 的匹配结果，
 * 用于捕获所有未匹配到任何已定义路由的 URL 路径。
 *
 * 布局结构：
 *   ┌──────────────────────────────┐
 *   │                              │
 *   │         404（大号装饰）      │
 *   │       "页面未找到"           │
 *   │    "您访问的页面不存在..."    │
 *   │                              │
 *   │  [← 返回首页]               │
 *   │                              │
 *   └──────────────────────────────┘
 *
 * 视觉风格：
 * - 大型 404 数字使用淡紫色（purple-200）作为装饰性元素
 * - 居中布局 + 纵向弹性间距（gap 体系）
 * - 返回按钮使用品牌紫色（purple-600），hover 加深效果
 * - 返回箭头 SVG 图标为内置内联图标，无需外部依赖
 *
 * @returns 404 错误提示页面组件
 */

/**
 * 404 页面未找到组件
 *
 * 功能：
 * - 显示 404 错误码和提示文字
 * - 提供返回首页的导航链接
 *
 * 使用场景：用户访问不存在的路由时显示
 */
import { Link } from 'react-router-dom';

function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center py-20 text-center">
      {/* ── 装饰层：大号 404 数字 ────────────────────────────── 大号 404 错误码 ── */}
      {/* 使用 text-8xl 和 font-bold 放大突出显示，purple-200 淡紫色
          作为视觉装饰而非可读文本，营造品牌氛围 */}
      <p className="text-8xl font-bold text-accent/20">404</p>
      {/* ── 信息层：主标题 + 副标题 ──────────────────────────── */}
      {/* 主标题：用户核心感知信息，深色粗体大字 */}
      <h1 className="mt-4 text-2xl font-semibold text-ink">
        页面未找到
      </h1>
      {/* 副标题：补充说明，灰色小字降低视觉权重 */}
      <p className="mt-2 text-text-tertiary">
        您访问的页面不存在或已被移除
      </p>
      {/* ── 操作层：返回首页引导按钮 ────────────────────────── 返回首页按钮 ── */}
      {/* 品牌紫色按钮，hover 加深；gap-2 控制图标与文字间距 */}
      <Link
        to="/"
        className="mt-8 inline-flex items-center gap-2 rounded-lg bg-accent px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-accent-hover"
      >
        <svg
          className="h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M10 19l-7-7m0 0l7-7m-7 7h18"
          />
        </svg>
        返回首页
      </Link>
    </div>
  );
}

export default NotFound;
