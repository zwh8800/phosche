/**
 * 照片详情页面（路由入口）
 *
 * 本组件是 /photo/* 路由对应的页面组件，负责从 URL 中提取照片路径参数，
 * 通过 API 加载照片详情数据，并根据请求状态（加载中/错误/成功）渲染不同 UI。
 *
 * 功能特性：
 * - 从 URL 通配符参数中解析照片文件路径（自动 URI 解码）
 * - 使用 React Query（useQuery）管理数据获取、缓存和自动重试
 * - 加载中状态显示居中的旋转加载动画（Tailwind spin 动画）
 * - 加载失败状态显示错误提示文本 + 重试按钮（调用 refetch 重新请求）
 * - 加载成功时渲染 PhotoDetailModal 模态弹窗展示照片完整信息
 * - 弹窗关闭时通过 navigate(-1) 返回上一级页面
 *
 * 路由格式：/photo/{path}，其中 {path} 为文件路径的 URI 编码值
 * 数据流：URL → useParams → decodeURIComponent → useQuery → fetchPhotoDetail → PhotoDetailModal
 * 状态机：idle → loading → success | error
 *
 * @returns 照片详情页面组件，渲染三种状态之一
 */

import { useParams, useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchPhotoDetail } from '../api/photos';
import PhotoDetailModal from '../components/PhotoDetail';

/**
 * 照片详情页组件
 *
 * 页面级别的路由组件，从 URL 路径参数中提取照片路径，通过 React Query
 * 加载照片详情数据，并根据请求状态（loading / error / success）分派到
 * 对应的 UI 分支：
 *   - 加载中 → 旋转加载动画
 *   - 加载失败 → 错误提示 + 重新加载按钮
 *   - 加载成功 → 渲染 PhotoDetailModal 模态弹窗
 *
 * 关闭弹窗时通过 navigate(-1) 返回上一页，保持与浏览器历史一致。
 */
function PhotoDetail() {
  // 从路由参数中提取通配符（*）匹配的原始路径片段（URI 编码状态）
  const { '*': wildcard } = useParams<{ '*': string }>();
  // 将 URI 编码的路径解码为实际文件系统路径；wildcard 为 undefined 时兜底为空字符串
  const id = wildcard ? decodeURIComponent(wildcard) : '';
  const navigate = useNavigate();

  // 使用 React Query 发起 API 请求获取照片详情
  // queryKey 包含 ['photo', id] 确保按照片 ID 独立缓存
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['photo', id],
    queryFn: () => fetchPhotoDetail(id!),
    enabled: !!id, // id 为空字符串时禁用请求，避免无效 API 调用
  });

  // ── 分支 1：加载中状态 ──────────────────────────────────────
  // 渲染居中的紫色旋转圆环加载动画，使用 Tailwind animate-spin
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-32">
        <div className="animate-spin rounded-full h-10 w-10 border-[3px] border-purple-200 border-t-purple-600" />
      </div>
    );
  }

  // ── 分支 2：错误或空数据状态 ────────────────────────────────
  // 显示"加载失败"提示和重试按钮；refetch() 会重新执行 queryFn
  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center py-32 gap-4">
        <p className="text-gray-500 text-lg">加载失败</p>
        <button
          onClick={() => refetch()}
          className="px-5 py-2.5 bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors text-sm font-medium cursor-pointer"
        >
          重试
        </button>
      </div>
    );
  }

  // ── 分支 3：成功加载状态 ────────────────────────────────────
  // 将照片数据传递给 PhotoDetailModal 组件渲染为模态弹窗
  // onClose 回调使用 navigate(-1) 实现"返回"语义（保持浏览器历史栈）
  return <PhotoDetailModal photo={data} onClose={() => navigate(-1)} />;
}

export default PhotoDetail;
