/**
 * 应用程序入口文件
 *
 * 负责初始化 React 根节点并渲染整个应用。
 * 使用 StrictMode 启用严格模式检查，帮助发现潜在的问题，
 * 例如废弃的 API 调用、副作用未正确清理等。
 *
 * 本文件是 Webpack/Vite 打包的入口点，在 index.html 中通过 <script type="module">
 * 引入。React 18 的 createRoot API 替代了旧的 ReactDOM.render，提供了更好的
 * 并发渲染能力和 hydration 支持。
 */
import { StrictMode } from 'react'                // React 严格模式组件，启用额外的开发期检查（双重渲染、废弃 API 警告等）
import { createRoot } from 'react-dom/client'       // React 18+ 新版根节点创建 API（取代旧的 ReactDOM.render），支持并发模式
import './index.css'                                 // 全局样式，包含 Tailwind CSS 的基础指令（@tailwind base/components/utilities）
import App from './App.tsx'                          // 应用根组件，包含路由和数据层配置

/**
 * 创建 React 根节点并渲染 App 组件
 *
 * 执行流程：
 * 1. 通过 document.getElementById 获取 index.html 中的挂载点（id="root" 的 <div>）
 * 2. 使用 createRoot 创建 React 18 并发模式的根实例
 * 3. 在 StrictMode 包裹下渲染 App 组件，StrictMode 会在开发环境执行额外的检查
 *    （检测不安全的生命周期、过时的 API 使用、副作用清理等）但不影响生产构建
 */
// '!' 非空断言：index.html 中确保存在 id 为 'root' 的 DOM 元素，运行时始终非空
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
