/**
 * 应用根组件
 *
 * 负责搭建应用的整体框架，包括：
 * - React Router 路由配置（时间线、搜索、照片详情、404 页面）
 * - TanStack React Query 数据请求客户端
 * - 全局错误边界处理
 *
 * 路由结构：
 * - /           → Timeline（时间线浏览，按日期分组展示照片）
 * - /search     → Search（多条件搜索页面）
 * - /photo/*    → PhotoDetail（照片详情页，* 为文件路径）
 * - *           → NotFound（404 页面）
 */
// ---- 路由 ----
import { createBrowserRouter, RouterProvider } from 'react-router-dom';  // 浏览器路由：基于 HTML5 History API 的声明式路由
// ---- 数据请求 ----
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'; // TanStack Query：服务端状态管理、缓存、后台刷新
// ---- 布局与错误处理 ----
import Layout from './components/Layout';            // 全局布局组件，包含顶部导航栏和移动端菜单
import ErrorBoundary from './components/ErrorBoundary'; // 错误边界，捕获子组件渲染异常并展示降级 UI
// ---- 页面组件 ----
import Timeline from './pages/Timeline';             // 时间线页面：按日期分组展示照片，支持无限滚动
import Search from './pages/Search';                 // 搜索页面：关键词 + 多条件筛选，结果同步到 URL 参数
import PhotoDetail from './pages/PhotoDetail';       // 照片详情页：展示 EXIF 元数据和 AI 分析结果
import NotFound from './pages/NotFound';             // 404 页面：未匹配路由时的兜底展示

/**
 * 创建 React Query 客户端实例
 * 用于管理所有 API 请求的缓存、重试和状态同步
 */
const queryClient = new QueryClient();

/**
 * 创建浏览器路由配置
 *
 * 所有页面都嵌套在 Layout 组件内，共享导航栏和页面布局。
 * 使用通配符路径 'photo/*' 匹配照片详情页，
 * 文件路径会作为通配符参数传递给 PhotoDetail 组件。
 */
const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { index: true, element: <Timeline /> },
      { path: 'search', element: <Search /> },
      { path: 'photo/*', element: <PhotoDetail /> },
      { path: '*', element: <NotFound /> },
    ],
  },
]);

/**
 * 应用根组件
 *
 * 渲染顺序（由外到内）：
 * 1. ErrorBoundary — 捕获渲染错误，显示友好的错误页面
 * 2. QueryClientProvider — 注入 React Query 上下文，支持数据缓存
 * 3. RouterProvider — 渲染路由匹配的页面组件
 */
function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
