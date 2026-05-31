/**
 * 时间线页面组件
 *
 * 按拍摄日期分组展示所有已导入照片，支持无限滚动加载。
 *
 * 功能特性：
 * - 按日期分组展示照片（useMemo 缓存分组结果，日期降序排列）
 * - 无限滚动加载（IntersectionObserver 监听哨兵元素，阈值 0.1）
 * - 滚动位置恢复（加载更多前后保持视图稳定，避免页面跳动）
 * - 加载状态骨架屏（SkeletonCard，Tailwind animate-pulse 脉冲动画）
 * - 错误状态与空状态处理
 * - 分析中照片显示脉冲动画占位
 *
 * 数据流：
 * useInfiniteQuery 分页获取（每页 50 张）
 * → pages.flatMap 合并所有页数据
 * → extractDate 提取拍摄日期（优先 EXIF，回退 mtime）
 * → Map 分组 → 日期降序排列 → 渲染日期分组网格
 *
 * 交互逻辑：
 * - 点击照片卡片 → navigate 跳转到 /photo/{path} 详情页
 * - 滚动至底部哨兵元素 → IntersectionObserver 触发 fetchNextPage
 *
 * 组件结构：
 * - SkeletonCard：加载中的骨架占位卡片（脉冲动画）
 * - PhotoCard：单张照片展示卡片（缩略图 + 描述 + 标签 + 状态标签）
 * - Timeline：主页面组件，管理无限滚动和分组逻辑
 */
import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { useRef, useEffect, useMemo } from 'react';
import { fetchPhotos } from '../api/photos';
import type { PhotoDocument } from '../types';

/**
 * 照片状态 → 中文标签映射表
 *
 * 状态值由后端流水线维护，映射为前端显示的中文文本，
 * 用于在照片卡片上展示人类可读的状态标签（右上角徽章）。
 * 状态流转：unanalyzed → analyzing → analyzed（成功）/ failed（失败）
 * 网络错误时进入 pending_analysis 队列等待重试。
 */
const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '失败',
  pending_analysis: '待分析',
  unanalyzed: '未分析',
};

/**
 * 照片状态 → Tailwind CSS 颜色类名映射
 *
 * 为不同状态分配语义化的背景色 + 文字颜色组合：
 * - analyzed:  绿色背景（成功/已完成，正向视觉反馈）
 * - analyzing: 黄色背景（进行中/处理中，提醒用户等待）
 * - failed:    红色背景（错误/失败，警示性视觉反馈）
 * - pending_analysis / unanalyzed: 灰色背景（等待/未处理，中性低调）
 */
const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-green-100 text-green-700',
  analyzing: 'bg-yellow-100 text-yellow-700',
  failed: 'bg-red-100 text-red-700',
  pending_analysis: 'bg-gray-100 text-gray-600',
  unanalyzed: 'bg-gray-100 text-gray-600',
};

/**
 * 从照片元数据中提取日期字符串（YYYY-MM-DD 格式）
 *
 * 优先使用 EXIF 拍摄时间（date_time_original），
 * 若不可用则回退到文件修改时间（mtime）。
 * 自动检测 mtime 是秒级还是毫秒级时间戳。
 *
 * @param photo - 照片文档对象
 * @returns 格式化的日期字符串，无法解析时返回 '未知日期'
 */
function extractDate(photo: PhotoDocument): string {
  // 优先尝试从 EXIF 数据提取拍摄日期
  const exifDate = photo.exif?.date_time_original;
  if (exifDate) {
    const d = new Date(exifDate);
    if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  }
  // 自动检测 unix 时间戳单位：大于 1e12 为毫秒，否则为秒
  const ts =
    photo.mtime > 1e12 ? photo.mtime : photo.mtime * 1000;
  const d = new Date(ts);
  if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  return '未知日期';
}

/**
 * 将日期字符串格式化为中文显示格式
 *
 * @param dateStr - YYYY-MM-DD 格式的日期字符串
 * @returns 中文格式，如 "2024年1月15日"
 */
function formatDateLabel(dateStr: string): string {
  const parts = dateStr.split('-');
  if (parts.length !== 3) return dateStr;
  return `${parts[0]}年${Number.parseInt(parts[1], 10)}月${Number.parseInt(parts[2], 10)}日`;
}

/**
 * 将 ISO 日期字符串格式化为中文日期标签
 *
 * 输入：'2024-01-15' → 输出：'2024年1月15日'
 * 用于在日期分组标题中以用户友好的中文格式展示日期
 *
 * @param dateStr - ISO 格式日期字符串 (YYYY-MM-DD)
 * @returns 中文格式的日期标签，如 "2024年1月15日"
 */
/**
 * 将日期字符串格式化为中文显示格式
 *
 * 拆分 YYYY-MM-DD 格式字符串，重组为中文日期表达。
 * 月/日部分去掉前导零（parseInt 后自动处理）。
 *
 * @param dateStr - YYYY-MM-DD 格式的日期字符串
 * @returns 中文格式，如 "2024年1月15日"
 */
function photoSrc(path: string): string {
  return `/photos/${path.replace(/^\/+/, '')}`;
}

/**
 * 骨架屏占位卡片组件
 *
 * 在数据加载期间显示脉冲动画的灰色占位块，
 * 模拟真实卡片的布局（正方形图片 + 两行文字）。
 */
function SkeletonCard() {
  return (
    <div className="animate-pulse">
      <div className="aspect-square rounded-lg bg-gray-200" />
      <div className="mt-2 space-y-1.5">
        <div className="h-3 w-3/4 rounded bg-gray-200" />
        <div className="h-3 w-1/2 rounded bg-gray-200" />
      </div>
    </div>
  );
}

/**
 * 单张照片展示卡片组件
 *
 * 功能：
 * - 显示照片缩略图（带懒加载）
 * - 图片加载失败时显示占位文字
 * - 分析中的照片显示状态标签和骨架屏描述
 * - 已分析的照片显示 AI 生成的描述和标签（最多 3 个）
 * - 点击跳转到照片详情页
 *
 * @param photo - 照片文档对象，包含路径、描述、标签、状态等信息
 */
function PhotoCard({ photo }: { photo: PhotoDocument }) {
  const navigate = useNavigate();

  return (
    <button
      type="button"
      onClick={() => navigate(`/photo/${encodeURIComponent(photo.path)}`)}
      className="group cursor-pointer text-left"
    >
      <div className="aspect-square overflow-hidden rounded-lg bg-gray-100 relative">
        {/* 照片缩略图，懒加载；加载失败时隐藏 img 并显示占位文字 */}
        <img
          src={`${photoSrc(photo.path)}?thumb=1`}
          alt={photo.description || '照片'}
          className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          loading="lazy"
          onError={(e) => {
            const el = e.currentTarget;
            el.style.display = 'none';
            el.nextElementSibling?.classList.remove('hidden');
          }}
        />
        <div className="hidden absolute inset-0 flex items-center justify-center bg-gray-100 text-gray-400 text-sm">
          无法加载图片
        </div>
        {/* 分析中的照片显示右上角状态标签 */}
        {photo.status === 'analyzing' && (
          <span className={`absolute top-2 right-2 text-[10px] px-1.5 py-0.5 rounded font-medium ${STATUS_COLORS.analyzing}`}>
            {STATUS_LABELS.analyzing}
          </span>
        )}
      </div>

      <div className="mt-2 space-y-1.5">
        {/* 分析中：显示骨架屏占位；已分析：显示描述和标签 */}
        {photo.status === 'analyzing' ? (
          <>
            <div className="animate-pulse space-y-1.5">
              <div className="h-3 w-full rounded bg-gray-200" />
              <div className="h-3 w-3/4 rounded bg-gray-200" />
            </div>
            <div className="animate-pulse flex flex-wrap gap-1">
              <div className="h-5 w-10 rounded bg-gray-200" />
              <div className="h-5 w-12 rounded bg-gray-200" />
              <div className="h-5 w-8 rounded bg-gray-200" />
            </div>
          </>
        ) : (
          <>
            {/* AI 生成的照片描述，最多显示 2 行 */}
            {photo.description && (
              <p className="line-clamp-2 text-sm leading-snug text-gray-700">
                {photo.description}
              </p>
            )}
            {/* 标签列表，最多显示 3 个，超出部分显示 +N */}
            {photo.tags && photo.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {photo.tags.slice(0, 3).map((tag) => (
                  <span
                    key={tag}
                    className="inline-block rounded bg-purple-50 px-1.5 py-0.5 text-xs text-purple-700"
                  >
                    {tag}
                  </span>
                ))}
                {photo.tags.length > 3 && (
                  <span className="text-xs text-gray-400">
                    +{photo.tags.length - 3}
                  </span>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </button>
  );
}

/**
 * 时间线主页面组件
 *
 * 核心功能：
 * - 使用 useInfiniteQuery 分页加载照片数据
 * - IntersectionObserver 实现无限滚动
 * - 按拍摄日期对照片进行分组，日期降序排列
 * - 滚动位置记忆：加载下一页时保存滚动位置，数据到达后恢复
 * - 三种状态渲染：加载中（骨架屏）、错误、空数据
 *
 * 数据流：
 * useInfiniteQuery(pages) → flatMap 合并所有页 → extractDate 分组 → 渲染
 */
export default function Timeline() {
  // 无限滚动哨兵元素引用，IntersectionObserver 监听此元素
  const sentinelRef = useRef<HTMLDivElement>(null);
  // 记录加载下一页前的滚动位置，用于数据到达后恢复
  const scrollPositionRef = useRef<number>(0);

  /**
   * useInfiniteQuery 配置：
   * - queryKey: 'photos' 用于缓存和自动失效
   * - getNextPageParam: 根据 total 和 page_size 计算是否有下一页
   * - 每页加载 50 张照片
   */
  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    error,
  } = useInfiniteQuery({
    queryKey: ['photos'],
    queryFn: ({ pageParam = 1 }) =>
      fetchPhotos({ page: pageParam, page_size: 50 }),
    getNextPageParam: (lastPage) => {
      const totalPages = Math.ceil(lastPage.total / lastPage.page_size);
      return lastPage.page < totalPages ? lastPage.page + 1 : undefined;
    },
    initialPageParam: 1,
  });

  /**
   * 无限滚动实现：
   * 使用 IntersectionObserver 监听哨兵元素，
   * 当哨兵进入视口（阈值 10%）且有下一页且未在加载中时，
   * 记录当前滚动位置并触发加载下一页。
   */
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          // 保存滚动位置，防止加载新内容后页面跳动
          scrollPositionRef.current = window.scrollY;
          fetchNextPage();
        }
      },
      { threshold: 0.1 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  /**
   * 滚动位置恢复：
   * 新数据加载完成后，恢复到之前保存的滚动位置，
   * 避免因内容高度变化导致的页面跳动。
   */
  useEffect(() => {
    if (scrollPositionRef.current > 0 && !isFetchingNextPage) {
      window.scrollTo(0, scrollPositionRef.current);
      scrollPositionRef.current = 0;
    }
  }, [data, isFetchingNextPage]);

  /**
   * 按日期分组照片：
   * 1. 将所有分页的照片合并为一个数组
   * 2. 使用 extractDate 提取每张照片的日期
   * 3. 按日期分组到 Map 中
   * 4. 按日期降序排列（最新的在前）
   */
  const groupedPhotos = useMemo(() => {
    const allPhotos = data?.pages.flatMap((page) => page.photos) ?? [];
    const groups = new Map<string, PhotoDocument[]>();

    for (const photo of allPhotos) {
      const dateStr = extractDate(photo);
      const bucket = groups.get(dateStr);
      if (bucket) {
        bucket.push(photo);
      } else {
        groups.set(dateStr, [photo]);
      }
    }

    return [...groups.entries()].sort(([a], [b]) => b.localeCompare(a));
  }, [data]);

  // 错误状态：显示错误图标 + 错误信息
  // 错误状态：显示错误图标和错误信息
  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-gray-500">
        <svg
          className="mb-4 h-16 w-16 text-gray-300"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1.5}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"
          />
        </svg>
        <p className="text-lg font-medium">加载失败</p>
        <p className="mt-1 text-sm">
          {(error as Error)?.message || '请稍后重试'}
        </p>
      </div>
    );
  }

  // 加载中状态：显示 6 个骨架屏卡片
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  // 空数据状态：显示引导文字
  if (groupedPhotos.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-gray-400">
        <svg
          className="mb-6 h-24 w-24 text-gray-200"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1}
            d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
          />
        </svg>
        <p className="text-xl font-medium text-gray-500">还没有照片</p>
        <p className="mt-2 text-sm text-gray-400">
          导入照片后将在此处按时间线展示
        </p>
      </div>
    );
  }

  // 主渲染：按日期分组的照片网格
  return (
    <div className="space-y-8">
      {/* 按日期分组渲染，每个日期一个 section */}
      {groupedPhotos.map(([dateStr, photos]) => (
        <section key={dateStr}>
          {/* 日期标题，sticky 定位在顶部，半透明背景 + 毛玻璃效果 */}
          <h2 className="sticky top-0 z-10 mb-4 bg-gray-50/90 py-2 text-lg font-semibold text-gray-800 backdrop-blur-sm">
            {formatDateLabel(dateStr)}
          </h2>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
            {photos.map((photo) => (
              <PhotoCard key={photo.id} photo={photo} />
            ))}
          </div>
        </section>
      ))}

      {/* 无限滚动哨兵元素，进入视口时触发加载下一页 */}
      <div ref={sentinelRef} className="h-4" />

      {/* 加载下一页时显示骨架屏 */}
      {isFetchingNextPage && (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={`loading-${i}`} />
          ))}
        </div>
      )}

      {/* 全部加载完毕提示 */}
      {!hasNextPage && groupedPhotos.length > 0 && (
        <p className="py-8 text-center text-sm text-gray-400">已加载全部照片</p>
      )}
    </div>
  );
}
