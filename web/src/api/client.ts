/**
 * @file Axios HTTP 客户端配置模块
 *
 * 创建并导出统一的 Axios 实例，供所有 API 调用使用。
 * - 浏览器环境使用相对路径 '/api'（由 Vite 代理或生产环境同源服务处理）
 * - SSR 环境使用绝对路径 'http://localhost:8080/api'
 * - 请求超时时间 10 秒（10000ms）
 * - 使用 fetch 适配器替代默认的 XMLHttpRequest
 * - 响应拦截器统一处理后端错误格式
 */

import axios from 'axios';

/**
 * API 基础路径
 *
 * 根据运行环境自动选择正确的 API 地址：
 * - 浏览器环境：使用相对路径 /api，由 Vite 开发服务器代理转发到后端 localhost:8080
 * - 服务端渲染（SSR）/ Node.js 环境：使用完整的 localhost:8080 地址直连后端
 */
const BASE_URL = typeof window === 'undefined'
  ? 'http://localhost:8080/api'
  : '/api';

/**
 * Axios HTTP 客户端实例
 *
 * 全局唯一的 HTTP 请求客户端，所有 API 调用均通过此实例发起。
 * 统一处理请求地址、超时控制以及错误响应格式。
 *
 * @property baseURL - API 基础路径，根据环境自动选择
 * @property timeout - 请求超时时间（10 秒），由 AGENTS.md 约定为调优值
 * @property adapter - 使用 fetch 适配器，兼容浏览器和服务端运行环境
 */
const apiClient = axios.create({
  baseURL: BASE_URL,
  timeout: 10000,
  adapter: 'fetch',
});

// 响应拦截器：统一提取并格式化错误信息
// - 正常响应（2xx）：直接透传，不做额外处理
// - 错误响应：优先从服务端返回体中提取 error.message，兜底使用 axios 原始错误信息
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    const msg = error.response?.data?.error?.message || error.message;
    return Promise.reject(new Error(msg));
  },
);

/** Axios HTTP 客户端实例，供 api/photos.ts 等所有 API 调用模块复用 */
export default apiClient;
