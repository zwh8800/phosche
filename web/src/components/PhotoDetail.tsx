/**
 * 照片详情模态框组件
 *
 * 使用 React Portal 在页面最顶层渲染照片详情弹窗，覆盖在原始页面之上。
 * 通过键盘方向键（←/→）支持上一张/下一张导航，Escape 键关闭弹窗。
 *
 * 布局结构：
 * - 半透明黑色遮罩层，模糊背景（backdrop-blur）
 * - 居中弹窗卡片：左侧为照片显示区（55%），右侧为信息面板（45%）
 * - 信息面板分三个区域：AI 分析、拍摄信息（EXIF）、文件信息
 *
 * 技术要点：
 * - 使用 createPortal 将 DOM 渲染到 document.body 下，避免父容器层叠上下文限制
 * - 弹窗打开时锁定 body 滚动（overflow: hidden），关闭时恢复原始值
 * - 键盘事件监听在 useEffect 中注册/清理
 * - 点击遮罩层关闭弹窗，点击弹窗内部阻止事件冒泡
 * - 弹窗入场动画通过动态注入 @keyframes 实现
 */
import { useEffect, useCallback, useState } from 'react';
import { createPortal } from 'react-dom';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import type { PhotoDocument } from '../types';
import { fetchSimilarPhotos, fetchNearbyPhotos } from '../api/photos';

/**
 * 照片详情模态框的属性接口
 *
 * @property photo - 当前展示的照片文档对象，包含 EXIF、AI 分析结果等完整数据
 * @property onClose - 关闭弹窗的回调函数，由父组件提供
 * @property onPrev - 切换到上一张照片的回调函数，可选；不提供时隐藏上一张按钮
 * @property onNext - 切换到下一张照片的回调函数，可选；不提供时隐藏下一张按钮
 * @property hasPrev - 是否存在上一张照片，控制上一张按钮的显示/隐藏
 * @property hasNext - 是否存在下一张照片，控制下一张按钮的显示/隐藏
 * @property onTagClick - 点击标签时的回调，可选；不提供时标签不可点击
 * @property onLocationClick - 点击地理位置时的回调，可选；不提供时位置不可点击
 */
interface PhotoDetailModalProps {
  photo: PhotoDocument;
  onClose: () => void;
  onPrev?: () => void;
  onNext?: () => void;
  hasPrev?: boolean;
  hasNext?: boolean;
  onTagClick?: (tag: string) => void;
  onLocationClick?: (text: string, params: { city?: string; district?: string; province?: string; country?: string }) => void;
}

/**
 * 标签颜色调色板
 *
 * 为 AI 识别的标签提供 8 种不同的颜色方案，通过 tagColor 哈希函数
 * 根据标签文字内容均匀分配颜色，确保同一标签始终显示相同颜色。
 * 每个色组包含背景色（-100）和文字色（-700）两个 Tailwind 类。
 */
const TAG_COLORS = [
  'bg-red-100 text-red-700',
  'bg-amber-100 text-amber-700',
  'bg-emerald-100 text-emerald-700',
  'bg-sky-100 text-sky-700',
  'bg-orange-100 text-orange-700',
  'bg-rose-100 text-rose-700',
  'bg-teal-100 text-teal-700',
  'bg-pink-100 text-pink-700',
];

/**
 * 处理状态对应的颜色映射表
 *
 * - analyzed: 绿色，表示分析成功
 * - analyzing: 黄色，表示正在分析中
 * - failed: 红色，表示分析失败
 * - pending_analysis: 灰色，等待重试
 * - unanalyzed: 灰色，尚未开始分析
 */
const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-green-100 text-green-700',
  analyzing: 'bg-yellow-100 text-yellow-700',
  failed: 'bg-red-100 text-red-700',
  pending_analysis: 'bg-gray-100 text-gray-600',
  unanalyzed: 'bg-gray-100 text-gray-600',
};

/**
 * 处理状态对应的中文标签映射表
 */
const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '分析失败',
  pending_analysis: '等待分析',
  unanalyzed: '未分析',
};

/**
 * 场景类型对应的中文标签映射表
 *
 * 覆盖 outdoor（室外）、indoor（室内）、underwater（水下）、
 * aerial（航拍）、studio（影棚）、night（夜景）、unknown（未知）七种场景。
 */
const SCENE_TYPE_LABELS: Record<string, string> = {
  outdoor: '室外',
  indoor: '室内',
  underwater: '水下',
  aerial: '航拍',
  studio: '影棚',
  night: '夜景',
  unknown: '未知',
};

/**
 * 标签颜色哈希分配函数
 *
 * 基于标签字符串内容计算哈希值，从 TAG_COLORS 调色板中均匀选取颜色。
 * 使用 DJB2 哈希算法（由 Dan Bernstein 发明），具有分布均匀、计算快速的特点。
 * 同一标签始终返回相同颜色，保证视觉一致性。
 *
 * @param tag - 标签文字
 * @returns Tailwind CSS 颜色类名，格式如 "bg-rose-100 text-rose-700"
 */
function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

/**
 * 格式化光圈值为标准显示格式
 *
 * - 如果值已包含 "f/" 或 "F/" 前缀，直接返回原值
 * - 如果是纯数字字符串，格式化为 "f/{数字}" 形式
 * - 其他情况（如异常值），原样返回
 *
 * @param aperture - 光圈原始值，如 "1.8" 或 "f/1.8"
 * @returns 格式化后的光圈字符串，如 "f/1.8"
 */
function formatAperture(aperture: string): string {
  if (aperture.startsWith('f/') || aperture.startsWith('F/')) return aperture;
  const num = parseFloat(aperture);
  if (!isNaN(num)) return `f/${num}`;
  return aperture;
}

/**
 * 格式化文件大小为人类可读格式
 *
 * 根据字节数自动选择单位：
 * - ≥ 1 MB：以 MB 为单位保留一位小数
 * - ≥ 1 KB：以 KB 为单位保留一位小数
 * - < 1 KB：以 B 为单位显示整数字节数
 *
 * @param bytes - 文件字节数
 * @returns 格式化后的文件大小字符串，如 "3.7 MB"、"128.0 KB"、"512 B"
 */
function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

/**
 * 将 Unix 时间戳格式化为中文日期时间字符串
 *
 * 输入为 Unix 秒级时间戳（非毫秒级），使用 toLocaleString 转换为
 * 中文格式：YYYY/MM/DD HH:mm:ss。
 *
 * @param ts - Unix 秒级时间戳
 * @returns 中文格式化日期字符串，如 "2024/01/15 14:30:00"
 */
function formatTimestamp(ts: number): string {
  return new Date(ts * 1000).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/**
 * 剪贴板复制按钮组件
 *
 * 点击时将指定文本复制到系统剪贴板，复制成功后显示对勾图标反馈，
 * 1.5 秒后自动恢复为复制图标。使用 navigator.clipboard.writeText API。
 *
 * @param props.text - 需要复制到剪贴板的文本内容（如文件路径）
 */
function CopyButton({ text }: { text: string }) {
  /** 复制成功状态标志，true 时显示勾号图标，1.5 秒后自动重置 */
  const [copied, setCopied] = useState(false);

  /**
   * 复制操作处理函数
   * 调用 clipboard API 写入文本，成功后设置 copied 状态为 true，
   * 并在 1.5 秒后自动重置。失败时静默忽略。
   */
  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // fallback: ignore
    }
  }, [text]);

  return (
    <button
      onClick={handleCopy}
      className="flex-shrink-0 p-1 rounded hover:bg-gray-200 transition-colors cursor-pointer"
      title={copied ? '已复制' : '复制'}
      aria-label="复制"
    >
      {/* 根据 copied 状态切换显示勾号图标或复制图标 */}
      {copied ? (
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M2 6l3 3 5-6" />
        </svg>
      ) : (
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <rect x="4" y="4" width="7" height="7" rx="1" />
          <path d="M8 4V2.5A0.5 0.5 0 0 0 7.5 2H2.5A0.5 0.5 0 0 0 2 2.5V7.5A0.5 0.5 0 0 0 2.5 8H4" />
        </svg>
      )}
    </button>
  );
}

/**
 * 场景类型徽章组件
 *
 * 根据场景类型标识符显示对应的 emoji 图标和中文标签。
 * 内置七种场景映射：outdoor（🌳室外）、indoor（🏠室内）、
 * underwater（🌊水下）、aerial（🛩️航拍）、studio（🎬影棚）、
 * night（🌙夜景）、unknown（🤷未知）。未知类型默认显示 📷 图标。
 *
 * @param props.type - 场景类型标识符，如 "outdoor"、"indoor"
 */
function SceneTypeBadge({ type }: { type: string }) {
  /** 场景类型到 emoji 的映射表 */
  const emoji: Record<string, string> = {
    outdoor: '🌳', indoor: '🏠', underwater: '🌊',
    aerial: '🛩️', studio: '🎬', night: '🌙', unknown: '🤷',
  };
  const label = SCENE_TYPE_LABELS[type] || type;

  return (
    <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-xs font-medium">
      {emoji[type] || '📷'} {label}
    </span>
  );
}

/**
 * 照片详情模态框主组件
 *
 * 使用 React Portal 渲染到 document.body，展示照片大图及完整的元数据信息。
 * 布局分为左（55% 图片区）右（45% 信息面板）两部分：
 *   - 图片区：深色背景，图片自适应居中，覆盖下载/关闭/上一张/下一张按钮
 *   - 信息面板：三个信息区块——AI 分析、拍摄信息（EXIF）、文件信息
 *
 * 键盘导航：
 *   - Escape：关闭弹窗
 *   - ArrowLeft：切换到上一张照片（需 hasPrev 为 true）
 *   - ArrowRight：切换到下一张照片（需 hasNext 为 true）
 *
 * @param props.photo  - 当前展示的照片文档对象
 * @param props.onClose - 关闭弹窗回调
 * @param props.onPrev  - 切换到上一张的回调（可选）
 * @param props.onNext  - 切换到下一张的回调（可选）
 * @param props.hasPrev - 是否存在上一张（可选）
 * @param props.hasNext - 是否存在下一张（可选）
 */
function PhotoDetailModal({
  photo,
  onClose,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
  onTagClick,
  onLocationClick,
}: PhotoDetailModalProps) {
  // 键盘导航处理：Escape 键关闭弹窗，←/→ 键切换照片
  /**
   * 键盘事件处理器（使用 useCallback 优化性能）
   *
   * 支持三个键盘快捷键：
   * - Escape：关闭弹窗，调用 onClose
   * - ArrowLeft（←）：切换到上一张，需 hasPrev 和 onPrev 同时存在
   * - ArrowRight（→）：切换到下一张，需 hasNext 和 onNext 同时存在
   *
   * 依赖项为 onClose/onPrev/onNext/hasPrev/hasNext，任一变化时重新创建函数引用，
   * 确保 useEffect 中的事件监听器始终绑定最新的回调。
   */
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      if (e.key === 'ArrowLeft' && hasPrev && onPrev) onPrev();
      if (e.key === 'ArrowRight' && hasNext && onNext) onNext();
    },
    [onClose, onPrev, onNext, hasPrev, hasNext],
  );

  // 注册键盘事件监听和 body 滚动锁定，组件卸载时自动清理
  /**
   * 副作用：弹窗打开/关闭时的全局状态管理
   *
   * 挂载时：
   * 1. 注册全局键盘事件监听器，支持键盘导航
   * 2. 保存当前 body overflow 值，然后设置为 'hidden' 锁定背景滚动
   *
   * 卸载时（清理函数）：
   * 1. 移除键盘事件监听器
   * 2. 恢复 body overflow 为之前保存的值，恢复页面滚动
   */
  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = prev;
    };
  }, [handleKeyDown]);

  // 构造照片原始文件 URL（去除路径前导斜杠）
  const imageUrl = `/photos/${photo.path.replace(/^\/+/, '')}`;
  // displayUrl 追加 ?convert=1，后端将 HEIC 等格式转换为 JPEG 供浏览器直接显示
  const displayUrl = `${imageUrl}?convert=1`;

  const [downloadToast, setDownloadToast] = useState(false);

  const { data: similarData } = useQuery({
    queryKey: ['similar', photo.id],
    queryFn: () => fetchSimilarPhotos(photo.id),
    enabled: photo.status === 'analyzed',
    staleTime: 5 * 60 * 1000,
  });

  const { data: nearbyData } = useQuery({
    queryKey: ['nearby', photo.id],
    queryFn: () => fetchNearbyPhotos(photo.id),
    enabled: photo.status === 'analyzed' && !!photo.exif?.gps_lat,
    staleTime: 5 * 60 * 1000,
  });

  const handleDownload = async () => {
    try {
      const response = await fetch(imageUrl);
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = photo.path.split('/').pop() || 'photo.jpg';
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);

      setDownloadToast(true);
      setTimeout(() => setDownloadToast(false), 3000);
    } catch (error) {
      console.error('下载失败:', error);
      alert('下载失败，请稍后重试');
    }
  };

  // 格式化 EXIF 拍摄时间为中文完整日期（年月日 + 星期 + 时分）
  const dateStr = photo.exif?.date_time_original
    ? new Date(photo.exif.date_time_original).toLocaleDateString('zh-CN', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        weekday: 'long',
        hour: '2-digit',
        minute: '2-digit',
      })
    : null;

  // 判断是否有可供展示的 EXIF 数据：任一关键字段（相机、镜头、焦距、光圈、快门、ISO、GPS）存在即视为有
  const hasExif =
    photo.exif &&
    (photo.exif.camera_model ||
      photo.exif.lens_model ||
      photo.exif.focal_length ||
      photo.exif.aperture ||
      photo.exif.shutter_speed ||
      photo.exif.iso ||
      photo.exif.date_time_original ||
      photo.exif.gps_lat != null);

  // 判断是否有 AI 分析结果数据：描述、标签、物体、场景类型、颜色中任一字段存在即有分析内容
  const hasAnalysis =
    photo.description ||
    photo.tags?.length > 0 ||
    photo.objects?.length > 0 ||
    photo.scene_type ||
    photo.colors?.length > 0;

  // 使用 createPortal 将弹窗渲染到 document.body 下，避免被父容器的层叠上下文和 overflow 限制
  return createPortal(
    <div className="fixed inset-0 z-50" role="dialog" aria-modal="true">
      <div
        className="absolute inset-0 bg-black/75 backdrop-blur-xs"
      />

      <div
        className="relative z-10 flex items-center justify-center w-full h-full p-3 sm:p-6"
        onClick={onClose}
      >
        <div
          className="relative flex flex-col lg:flex-row w-full max-w-[1400px] max-h-[92vh] bg-white rounded-2xl shadow-2xl overflow-hidden animate-[fadeIn_0.2s_ease-out]"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="absolute top-4 right-4 z-20 flex items-center gap-2">
            <button
              onClick={handleDownload}
              className="flex items-center justify-center w-9 h-9 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              title="下载原图"
              aria-label="下载原图"
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M8 2v8" />
                <path d="M4 7l4 4 4-4" />
                <path d="M2 12v1.5A0.5 0.5 0 0 0 2.5 14h11a0.5 0.5 0 0 0 .5-.5V12" />
              </svg>
            </button>

            <button
              onClick={onClose}
              className="flex items-center justify-center w-9 h-9 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="关闭"
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 18 18"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              >
                <path d="M4 4l10 10M14 4L4 14" />
              </svg>
            </button>
          </div>

          {hasPrev && (
            <button
              onClick={onPrev}
              className="absolute top-1/2 -translate-y-1/2 left-3 z-20 flex items-center justify-center w-10 h-10 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="上一张"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M12 4l-6 6 6 6" />
              </svg>
            </button>
          )}
          {hasNext && (
            <button
              onClick={onNext}
              className="absolute top-1/2 -translate-y-1/2 right-3 z-20 flex items-center justify-center w-10 h-10 rounded-full bg-black/20 hover:bg-black/40 text-white backdrop-blur-sm transition-colors cursor-pointer"
              aria-label="下一张"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 20 20"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M8 4l6 6-6 6" />
              </svg>
            </button>
          )}

          {/* ── Image area ── 照片显示区域 ── */}
          <div className="relative flex items-center justify-center bg-gray-900 lg:w-[55%] min-h-[280px] lg:min-h-0 max-h-[50vh] lg:max-h-full overflow-hidden shrink-0">
            {/*
             * 图片加载失败时隐藏 <img> 元素，显示相邻的 "无法加载图片" 占位文字。
             * onError 触发后：1) 设置 img display:none；2) 显示相邻的 fallback div
             */}
            <img
              src={displayUrl}
              alt={photo.description || '照片'}
              className="w-full h-full object-contain"
              loading="eager"
              onError={(e) => {
                const el = e.currentTarget;
                el.style.display = 'none';
                el.nextElementSibling?.classList.remove('hidden');
              }}
            />
            {/* 图片加载失败时的占位提示（默认 hidden，通过 onError 显示） */}
            <div className="hidden absolute inset-0 flex items-center justify-center bg-gray-900 text-gray-400 text-sm">
              无法加载图片
            </div>
          </div>

          {/* ── Sidebar ── 右侧信息面板 ── */}
          <div className="flex flex-col lg:w-[45%] overflow-y-auto">
            <div className="p-6 lg:p-8 space-y-6">
              {/* ── Section 1: AI 分析 ── AI 分析结果 ── */}
              {/*
               * AI 分析区域：条件渲染。
               * hasAnalysis 为 true 时展示完整的分析结果（描述、场景、标签、颜色、物体、文字、置信度）；
               * hasAnalysis 为 false 时根据 photo.status 显示不同的状态提示文案。
               */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  AI 分析
                </h3>

                {hasAnalysis ? (
                  <>
                    {photo.description && (
                      <p className="text-base lg:text-lg font-medium text-gray-900 leading-relaxed">
                        {photo.description}
                      </p>
                    )}

                    {/*
                     * 元信息行：场景类型 + 人数 + 含文字标记
                     * 三者任一存在时整行渲染，各自独立条件控制显示
                     */}
                    {(photo.scene_type ||
                      photo.people_count > 0 ||
                      photo.has_text) && (
                      <div className="flex flex-wrap items-center gap-3 mt-4 mb-4">
                        {photo.scene_type && (
                          <SceneTypeBadge type={photo.scene_type} />
                        )}
                        {photo.people_count > 0 && (
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-xs font-medium">
                            <svg
                              width="14"
                              height="14"
                              viewBox="0 0 14 14"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.5"
                            >
                              <circle cx="5" cy="4.5" r="1.5" />
                              <circle cx="9" cy="4.5" r="1.5" />
                              <path d="M3 10c0-1.5 1.5-2.5 4-2.5s4 1 4 2.5" />
                            </svg>
                            {photo.people_count} 人
                          </span>
                        )}
                        {photo.has_text && (
                          <span className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-blue-50 text-blue-700 text-xs font-medium">
                            <svg
                              width="14"
                              height="14"
                              viewBox="0 0 14 14"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="1.5"
                            >
                              <path d="M2 3h10M4 6h6M4 9h4" />
                            </svg>
                            含文字
                          </span>
                        )}
                      </div>
                    )}

                     {/*
                      * 标签列表：使用 tagColor 哈希函数为每个标签分配确定性的颜色，
                      * 以圆角 pill 样式展示，视觉上区分不同类别的标签。
                      * 提供 onTagClick 时标签可点击，触发搜索该标签。
                      */}
                     {photo.tags?.length > 0 && (
                      <div className="flex flex-wrap gap-2 mb-4">
                        {photo.tags.map((t) => {
                          const tagCls = `inline-flex items-center px-2.5 py-1 rounded-full text-xs font-medium ${tagColor(t)}`;
                          if (onTagClick) {
                            return (
                              <button
                                key={t}
                                type="button"
                                onClick={() => onTagClick(t)}
                                className={`${tagCls} hover:opacity-80 cursor-pointer transition-opacity`}
                              >
                                {t}
                              </button>
                            );
                          }
                          return (
                            <span key={t} className={tagCls}>
                              {t}
                            </span>
                          );
                        })}
                      </div>
                    )}

                    {/*
                     * 主色调列表：圆形色块（使用后端返回的 hex 值作为 background-color）
                     * 配合颜色名称显示，直观展示照片的主要色彩倾向。
                     */}
                    {photo.colors?.length > 0 && (
                      <div className="mb-4">
                        <span className="text-xs text-gray-400 block mb-2">主色调</span>
                        <div className="flex flex-wrap gap-1.5">
                          {photo.colors.map((c) => (
                            <div key={c.name} className="flex items-center gap-1.5">
                              <div
                                className="w-5 h-5 rounded-full border border-gray-200 shadow-sm"
                                style={{ backgroundColor: c.hex }}
                              />
                              <span className="text-xs text-gray-500">{c.name}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}

                    {/*
                     * 画面物体列表：AI 从照片中检测到的物体或元素，
                     * 以灰色边框标签展示，区分于 AI 标签的彩色样式。
                     */}
                    {photo.objects?.length > 0 && (
                      <div className="mb-4">
                        <span className="text-xs text-gray-400 block mb-2">
                          画面物体
                        </span>
                        <div className="flex flex-wrap gap-2">
                          {photo.objects.map((obj) => (
                            <span
                              key={obj}
                              className="inline-flex items-center px-2.5 py-1 rounded-md bg-gray-50 text-gray-600 text-xs border border-gray-200"
                            >
                              {obj}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}

                    {/*
                     * 提取文字区域：AI 从照片中识别出的文字内容（OCR 结果），
                     * 仅在 has_text 为 true 且 text 字段非空时渲染。
                     */}
                    {photo.has_text && photo.text && (
                      <div className="mb-4 p-3 rounded-lg border border-gray-200 bg-gray-50">
                        <span className="text-xs text-gray-400 block mb-2">
                          提取文字
                        </span>
                        <p className="text-sm text-gray-700 leading-relaxed whitespace-pre-wrap break-words">
                          {photo.text}
                        </p>
                      </div>
                    )}

                    {/*
                     * 置信度：AI 分析结果的置信度（0~1 之间的小数），
                     * 转换为百分比显示，表征分析结果的可靠程度。
                     */}
                    {photo.confidence != null && (
                      <p className="text-xs text-gray-400">
                        置信度: {Math.round(photo.confidence * 100)}%
                      </p>
                    )}
                  </>
                ) : (
                  <>
                    {(photo.status === 'analyzing' || photo.status === 'failed') ? (
                      <div className="animate-pulse space-y-3">
                        <div className={`h-4 w-full rounded ${photo.status === 'failed' ? 'bg-red-200' : 'bg-gray-200'}`} />
                        <div className={`h-4 w-3/4 rounded ${photo.status === 'failed' ? 'bg-red-200' : 'bg-gray-200'}`} />
                        <div className={`h-4 w-1/2 rounded ${photo.status === 'failed' ? 'bg-red-200' : 'bg-gray-200'}`} />
                      </div>
                    ) : (
                      <p className="text-sm text-gray-400 italic">
                        {photo.status === 'unanalyzed'
                          ? '尚未分析'
                          : '暂无分析数据'}
                      </p>
                    )}
                  </>
                )}
              </section>

              {/* ── Section 2: 拍摄信息 ── EXIF 拍摄信息 ── */}
              {/*
               * 拍摄信息区域：条件渲染。
               * hasExif 为 true 时展示完整的 EXIF 元数据（相机型号、镜头、拍摄参数、GPS、地址）；
               * hasExif 为 false 时显示 "暂无 EXIF 数据" 提示。
               */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  拍摄信息
                </h3>

                {hasExif ? (
                  <>
                    {/*
                     * 相机 + 镜头型号组合：两者任一存在时整块渲染，
                     * 相机型号加粗显示，镜头型号以次级文字展示。
                     */}
                    {(photo.exif!.camera_model ||
                      photo.exif!.lens_model) && (
                      <div className="mb-4">
                        {photo.exif!.camera_model && (
                          <p className="text-sm font-semibold text-gray-900">
                            {photo.exif!.camera_model}
                          </p>
                        )}
                        {photo.exif!.lens_model && (
                          <p className="text-sm text-gray-500">
                            {photo.exif!.lens_model}
                          </p>
                        )}
                      </div>
                    )}

                    {/*
                     * 拍摄参数行：焦距 / 光圈（formatAperture 格式化）/ 快门速度 / ISO，
                     * 以灰色等宽字体徽章排列显示。
                     */}
                    {(photo.exif!.focal_length ||
                      photo.exif!.aperture ||
                      photo.exif!.shutter_speed ||
                      photo.exif!.iso != null) && (
                      <div className="flex flex-wrap gap-3 mb-4">
                        {photo.exif!.focal_length && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            {photo.exif!.focal_length}
                          </span>
                        )}
                        {photo.exif!.aperture && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            {formatAperture(photo.exif!.aperture)}
                          </span>
                        )}
                        {photo.exif!.shutter_speed && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            {photo.exif!.shutter_speed}
                          </span>
                        )}
                        {photo.exif!.iso != null && (
                          <span className="inline-flex items-center px-3 py-1.5 rounded-lg bg-gray-100 text-gray-700 text-sm font-mono">
                            ISO {photo.exif!.iso}
                          </span>
                        )}
                      </div>
                    )}

                    {/* 拍摄日期 */}
                    {dateStr && (
                      <div className="flex items-start gap-2 mb-2">
                        <svg className="w-4 h-4 text-gray-400 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" />
                        </svg>
                        <p className="text-sm text-gray-500">{dateStr}</p>
                      </div>
                    )}

                    {/*
                     * 格式化地址：优先展示高德逆地理编码返回的完整地址，
                     * 若无则拼接省/市/区三级行政区域名称。
                     * 第二行补充展示更细粒度的地理层级（国家/省/市/区 + 街道/门牌）。
                     * 第三行展示 GPS 十进制坐标（等宽字体）。
                     */}
                    {(photo.formatted_address || photo.city || (photo.exif?.gps_lat != null && photo.exif?.gps_lon != null)) && (
                      <div className="mt-2 space-y-0.5">
                        {(photo.formatted_address || photo.city) && (() => {
                          const locationText = photo.formatted_address || [photo.province, photo.city, photo.district].filter(Boolean).join(' ');
                          return (
                            <>
                              <div className="flex items-start gap-2 text-left">
                                <svg className="w-4 h-4 text-gray-400 mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M15 10.5a3 3 0 11-6 0 3 3 0 016 0z" />
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1115 0z" />
                                </svg>
                                {onLocationClick ? (
                                  <button
                                    type="button"
                                    onClick={() => onLocationClick(locationText, {
                                      city: photo.city,
                                      district: photo.district,
                                      province: photo.province,
                                      country: photo.country,
                                    })}
                                    className="text-sm text-gray-600 hover:text-gray-900 hover:underline cursor-pointer transition-colors text-left"
                                  >
                                    {locationText}
                                  </button>
                                ) : (
                                  <span className="text-sm text-gray-600 text-left">
                                    {locationText}
                                  </span>
                                )}
                              </div>
                            {[
                              [photo.country, photo.province, photo.city, photo.district].filter(Boolean).join(' '),
                              [photo.township, photo.business_area, photo.street, photo.street_number].filter(Boolean).join(' '),
                            ].filter(Boolean).join(' · ') && (
                              <p className="text-xs text-gray-400 pl-6">
                                {[
                                  [photo.country, photo.province, photo.city, photo.district].filter(Boolean).join(' '),
                                  [photo.township, photo.business_area, photo.street, photo.street_number].filter(Boolean).join(' '),
                                ].filter(Boolean).join(' · ')}
                              </p>
                            )}
                            </>
                          );
                        })()}
                        {photo.exif?.gps_lat != null && photo.exif?.gps_lon != null && (
                          <p className={`text-xs text-gray-400 font-mono ${(photo.formatted_address || photo.city) ? 'pl-6' : ''}`}>
                            {photo.exif.gps_lat.toFixed(6)}, {photo.exif.gps_lon.toFixed(6)}
                          </p>
                        )}
                      </div>
                    )}
                  </>
                ) : (
                  <p className="text-sm text-gray-400 italic">
                    暂无 EXIF 数据
                  </p>
                )}
              </section>

              {/* ── Section 3: 文件信息 ── */}
              {/*
               * 文件信息区域：始终渲染（文件元数据必然存在）。
               * 展示路径（带复制按钮）、大小、修改时间、创建时间、分析时间、处理状态。
               */}
              <section>
                <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                  文件信息
                </h3>

                <div className="space-y-2.5 text-sm">
                  {/*
                   * 文件路径行：等宽字体 + 自动换行（break-all），
                   * 右侧附带 CopyButton 组件便于复制路径。
                   */}
                  <div className="flex items-start gap-2">
                    <span className="text-gray-400 shrink-0">路径</span>
                    <span className="font-mono text-gray-600 break-all text-xs leading-relaxed flex-1">
                      {photo.path}
                    </span>
                    <CopyButton text={photo.path} />
                  </div>

                  {/*
                   * 文件大小：通过 formatSize 自动选择合适的单位（MB/KB/B）展示。
                   */}
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">大小</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatSize(photo.size)}
                    </span>
                  </div>

                  {/* 文件最后修改时间（Unix 时间戳格式化为中文日期） */}
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">修改时间</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatTimestamp(photo.mtime)}
                    </span>
                  </div>

                  {/* 文件创建时间（即照片被索引到 ES 的时间） */}
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">创建时间</span>
                    <span className="font-mono text-gray-700 text-sm">
                      {formatTimestamp(photo.created_at)}
                    </span>
                  </div>

                  {/*
                   * AI 分析时间：条件渲染，仅 analyzed 状态的照片有 analyzed_at 字段。
                   */}
                  {photo.analyzed_at != null && (
                    <div className="flex items-center gap-2">
                      <span className="text-gray-400 shrink-0">分析时间</span>
                      <span className="font-mono text-gray-700 text-sm">
                        {formatTimestamp(photo.analyzed_at)}
                      </span>
                    </div>
                  )}

                  {/*
                   * 处理状态：使用 STATUS_COLORS 映射颜色，STATUS_LABELS 映射中文标签，
                   * 颜色兜底使用 unanalyzed 的灰色样式，文字兜底显示原始状态字符串。
                   */}
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400 shrink-0">状态</span>
                    <span
                      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${STATUS_COLORS[photo.status] || STATUS_COLORS.unanalyzed}`}
                    >
                      {STATUS_LABELS[photo.status] || photo.status}
                    </span>
                  </div>
                </div>
              </section>

              {(() => {
                const similarPhotos = similarData?.photos;
                const nearbyPhotos = nearbyData?.photos;
                const hasSimilar = !!similarPhotos && similarPhotos.length > 0;
                const hasNearby = !!nearbyPhotos && nearbyPhotos.length > 0;
                if (!hasSimilar && !hasNearby) return null;
                return (
                  <section>
                    <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-4">
                      推荐
                    </h3>

                    {hasSimilar && (
                      <div className="mb-4">
                        <p className="text-xs text-gray-400 mb-2">相似照片</p>
                        <div className="flex gap-2">
                          {similarPhotos!.map((p) => (
                            <Link
                              key={p.id}
                              to={`/photo/${p.id}`}
                              className="block w-1/3 aspect-square rounded-lg overflow-hidden bg-gray-100"
                            >
                              <img
                                src={`/photos/${p.path.replace(/^\/+/, '')}?thumb=1`}
                                alt={p.description || '照片'}
                                className="w-full h-full object-cover hover:scale-105 transition-transform"
                                loading="lazy"
                                onError={(e) => {
                                  const el = e.currentTarget;
                                  el.style.display = 'none';
                                }}
                              />
                            </Link>
                          ))}
                        </div>
                      </div>
                    )}

                    {hasNearby && (
                      <div className="mb-4">
                        <p className="text-xs text-gray-400 mb-2">附近照片</p>
                        <div className="flex gap-2">
                          {nearbyPhotos!.map((p) => (
                            <Link
                              key={p.id}
                              to={`/photo/${p.id}`}
                              className="block w-1/3 aspect-square rounded-lg overflow-hidden bg-gray-100"
                            >
                              <img
                                src={`/photos/${p.path.replace(/^\/+/, '')}?thumb=1`}
                                alt={p.description || '照片'}
                                className="w-full h-full object-cover hover:scale-105 transition-transform"
                                loading="lazy"
                                onError={(e) => {
                                  const el = e.currentTarget;
                                  el.style.display = 'none';
                                }}
                              />
                            </Link>
                          ))}
                        </div>
                      </div>
                    )}
                  </section>
                );
              })()}
            </div>
          </div>
        </div>
      </div>

      {downloadToast && (
        <div className="fixed bottom-8 left-1/2 -translate-x-1/2 z-30 flex items-center gap-2 bg-green-600 text-white px-4 py-2.5 rounded-full shadow-lg animate-[fadeIn_0.2s_ease-out]">
          <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          <span className="text-sm font-medium">已开始下载</span>
        </div>
      )}

      {/*
       * 弹窗入场动画：通过内联 <style> 标签注入 @keyframes fadeIn。
       * 动画从透明 + 略微缩小（scale 0.97）过渡到完整大小和不透明度，
       * 持续 0.2 秒，ease-out 缓动实现快速进入后平滑停止。
       */}
      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: scale(0.97); }
          to { opacity: 1; transform: scale(1); }
        }
      `}</style>
    </div>,
    document.body,
  );
}

export default PhotoDetailModal;
