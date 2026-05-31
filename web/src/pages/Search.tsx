/**
 * 搜索页面
 *
 * 功能特性：
 * - 关键词全文搜索 + 多条件筛选
 * - 搜索条件和 URL 查询参数双向同步（useSearchParams）
 * - 筛选条件变化后 300ms 防抖自动搜索
 * - 无限滚动加载搜索结果
 * - 手动提交搜索（回车/点击按钮）
 * - 重置所有筛选条件
 *
 * 筛选项：
 * - 拍摄日期范围（date_from / date_to）
 * - 场景类型（scene_type）：室外/室内/水下/航拍等
 * - 相机型号（camera_model）
 * - 标签（tags）：多选，逗号分隔
 *
 * 数据流：
 * fetchFilters（获取筛选项）→ useInfiniteQuery（带筛选条件的搜索）→ 渲染结果网格
 *
 * 状态管理：
 * - URL 驱动：页面初始化时从 searchParams 恢复所有筛选条件
 * - 防抖搜索：任何筛选条件变化后等待 300ms 自动触发搜索
 * - 懒加载：searchActive 控制查询是否启用，首次搜索后才激活
 */

import {
  useState,
  useEffect,
  useRef,
  memo,
  type FormEvent,
  type ChangeEvent,
} from 'react';
import { useSearchParams, Link } from 'react-router-dom';
import { useQuery, useInfiniteQuery } from '@tanstack/react-query';
import { searchPhotos, fetchFilters } from '../api/photos';
import type {
  FiltersResponse,
  PhotoDocument,
} from '../types';

/** 每页搜索结果数量 */
const PAGE_SIZE = 20;

/**
 * 格式化时间戳为中文日期字符串
 *
 * @param ts - Unix 时间戳（秒）
 * @returns 中文格式日期（如 "2024年01月15日"）
 */
function formatMtime(ts?: number): string {
  if (!ts) return '';
  return new Date(ts * 1000).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  });
}

/**
 * 格式化 EXIF 日期字符串为 YYYY-MM-DD 格式
 *
 * EXIF 中的日期格式通常为 "2024:01:15 10:30:00"，此函数尝试解析后
 * 转为标准 ISO 格式（2024-01-15）。若解析失败则截取前 10 个字符。
 *
 * @param raw - EXIF 原始日期字符串
 * @returns 格式化后的日期字符串（如 "2024-01-15"），无效输入返回空串
 */
function formatExifDate(raw?: string): string {
  if (!raw) return '';
  const d = new Date(raw);
  if (!Number.isNaN(d.getTime())) return d.toISOString().slice(0, 10);
  return raw.slice(0, 10);
}

/**
 * 加载中旋转指示器
 *
 * 居中显示一个紫色旋转圆环，用于搜索结果加载时的视觉反馈。
 * 使用 Tailwind 的 animate-spin 类实现 CSS 旋转动画。
 */
function Spinner() {
  return (
    <div className="flex items-center justify-center py-12">
      <div className="w-8 h-8 border-3 border-gray-200 border-t-purple-500 rounded-full animate-spin" />
    </div>
  );
}

/**
 * 骨架屏卡片占位组件
 *
 * 在搜索结果加载过程中显示灰色占位块，模拟照片卡片的布局
 * （图片区域 + 两行文字），使用 animate-pulse 实现脉冲动画效果。
 */
function SkeletonCard() {
  return (
    <div className="animate-pulse">
      <div className="aspect-[4/3] rounded-xl bg-gray-200" />
      <div className="mt-2 space-y-1.5 p-1">
        <div className="h-3 w-3/4 rounded bg-gray-200" />
        <div className="h-3 w-1/2 rounded bg-gray-200" />
      </div>
    </div>
  );
}

/** 照片处理状态的中文标签映射，用于在卡片上显示状态徽章 */
const STATUS_LABELS: Record<string, string> = {
  analyzed: '已分析',
  analyzing: '分析中',
  failed: '失败',
  pending_analysis: '待分析',
  unanalyzed: '未分析',
};

/** 照片处理状态对应的 Tailwind 颜色类，用于状态徽章的背景色和文字色 */
const STATUS_COLORS: Record<string, string> = {
  analyzed: 'bg-green-100 text-green-700',
  analyzing: 'bg-yellow-100 text-yellow-700',
  failed: 'bg-red-100 text-red-700',
  pending_analysis: 'bg-gray-100 text-gray-600',
  unanalyzed: 'bg-gray-100 text-gray-600',
};

/** 场景类型的中文标签映射，用于在卡片和筛选器中显示场景名称 */
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
 * 照片卡片组件（使用 memo 优化渲染性能）
 *
 * 展示单张搜索结果照片的缩略图、拍摄日期、AI 分析描述、场景类型和标签。
 * 点击跳转到照片详情页（/photo/{path}）。
 *
 * 渲染逻辑：
 * - 图片加载失败时隐藏 <img>，显示"无法加载图片"占位文字
 * - 右上角显示处理状态徽章（颜色由 STATUS_COLORS 决定）
 * - 分析中状态（analyzing）显示骨架屏动画，等待分析完成
 * - 标签最多显示 3 个，超出数量显示 "+N" 提示
 *
 * @param photo - 照片文档数据，包含路径、EXIF、AI 分析结果等
 */
const PhotoCard = memo(function PhotoCard({ photo }: { photo: PhotoDocument }) {
  // 优先使用 EXIF 拍摄日期，回退到文件修改时间
  const dateLabel =
    formatExifDate(photo.exif?.date_time_original) ||
    formatMtime(photo.mtime);

  return (
    <Link
      to={`/photo/${encodeURIComponent(photo.path)}`}
      className="group block rounded-xl overflow-hidden bg-white border border-gray-200 shadow-sm hover:shadow-md transition-shadow"
    >
      <div className="relative aspect-[4/3] bg-gray-100 overflow-hidden">
        {/* 照片缩略图，加载失败时隐藏并显示占位文字 */}
        <img
          src={`/photos/${photo.path.replace(/^\/+/, '')}?thumb=1`}
          alt={photo.description || photo.path}
          loading="lazy"
          className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
          onError={(e) => {
            const el = e.currentTarget;
            el.style.display = 'none';
            el.nextElementSibling?.classList.remove('hidden');
          }}
        />
        <div className="hidden absolute inset-0 flex items-center justify-center bg-gray-100 text-gray-400 text-sm">
          无法加载图片
        </div>
        {/* 右上角状态标签 */}
        <span
          className={`absolute top-2 right-2 text-[10px] px-1.5 py-0.5 rounded font-medium ${STATUS_COLORS[photo.status] || STATUS_COLORS.unanalyzed}`}
        >
          {STATUS_LABELS[photo.status] || STATUS_LABELS.unanalyzed}
        </span>
      </div>
      <div className="p-3 space-y-2">
        {dateLabel && (
          <p className="text-xs text-gray-500">{dateLabel}</p>
        )}
        {/* 分析中：显示骨架屏占位；已分析：显示描述、场景类型和标签 */}
        {photo.status === 'analyzing' ? (
          <>
            <div className="animate-pulse space-y-2">
              <div className="h-3 w-full rounded bg-gray-200" />
              <div className="h-3 w-3/4 rounded bg-gray-200" />
            </div>
            <div className="animate-pulse">
              <div className="inline-block h-5 w-12 rounded-full bg-gray-200" />
            </div>
            <div className="animate-pulse flex gap-1">
              <div className="h-4 w-10 rounded bg-gray-200" />
              <div className="h-4 w-12 rounded bg-gray-200" />
              <div className="h-4 w-8 rounded bg-gray-200" />
            </div>
          </>
        ) : (
          <>
            {/* AI 生成的照片描述 */}
            {photo.description && (
              <p className="text-sm text-gray-800 line-clamp-2 leading-snug">
                {photo.description}
              </p>
            )}
            {/* 场景类型标签 */}
            {photo.scene_type && (
              <span className="inline-block text-[11px] px-2 py-0.5 bg-purple-50 text-purple-600 rounded-full font-medium">
                {SCENE_TYPE_LABELS[photo.scene_type] || photo.scene_type}
              </span>
            )}
            {/* 标签列表，最多显示 3 个 */}
            {photo.tags && photo.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {photo.tags.slice(0, 3).map((tag) => (
                  <span
                    key={tag}
                    className="text-[10px] px-1.5 py-px bg-gray-100 text-gray-600 rounded"
                  >
                    {tag}
                  </span>
                ))}
                {photo.tags.length > 3 && (
                  <span className="text-[10px] text-gray-400">
                    +{photo.tags.length - 3}
                  </span>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Link>
  );
});

/**
 * 搜索页面主组件
 *
 * 核心功能：
 * - 全文关键词搜索 + 多条件筛选
 * - URL 同步：所有筛选条件通过 useSearchParams 同步到 URL
 * - 防抖自动搜索：筛选条件变化后 300ms 自动触发搜索
 * - 无限滚动加载（IntersectionObserver）
 * - 筛选面板可折叠，显示活跃筛选数量
 *
 * 状态管理：
 * - useState: 管理各筛选条件（query, dateFrom, dateTo, sceneType, cameraModel, selectedTags）
 * - useSearchParams: URL 参数双向同步
 * - useInfiniteQuery: 搜索结果分页加载
 * - useQuery: 获取可用筛选选项（标签、场景类型、相机型号）
 *
 * URL 参数映射：
 * query → keywords, date_from → dateFrom, date_to → dateTo,
 * scene_type → sceneType, camera_model → cameraModel, tags → selectedTags
 */
export default function Search() {
  // URL 搜索参数管理，实现筛选条件的 URL 同步
  const [searchParams, setSearchParams] = useSearchParams();

  // 筛选条件状态，初始化时从 URL 参数读取
  const [query, setQuery] = useState(searchParams.get('query') || '');
  const [dateFrom, setDateFrom] = useState(searchParams.get('date_from') || '');
  const [dateTo, setDateTo] = useState(searchParams.get('date_to') || '');
  const [sceneType, setSceneType] = useState(searchParams.get('scene_type') || '');
  const [cameraModel, setCameraModel] = useState(searchParams.get('camera_model') || '');
  const [selectedTags, setSelectedTags] = useState<string[]>(() => {
    const raw = searchParams.get('tags');
    return raw ? raw.split(',').filter(Boolean) : [];
  });
  // 筛选面板展开/折叠状态
  const [showFilters, setShowFilters] = useState(false);
  // 搜索激活标志，控制 useInfiniteQuery 是否执行
  const [searchActive, setSearchActive] = useState(false);
  // 无限滚动哨兵元素引用
  const sentinelRef = useRef<HTMLDivElement>(null);
  // 标记是否为首次挂载，首次挂载时根据 URL 参数决定是否自动搜索
  const initialMount = useRef(true);

  /**
   * 获取可用筛选选项（标签、场景类型、相机型号）
   * 缓存 5 分钟，避免重复请求
   */
  const { data: filters } = useQuery<FiltersResponse>({
    queryKey: ['filters'],
    queryFn: fetchFilters,
    staleTime: 5 * 60 * 1000,
  });

  /**
   * 搜索结果无限查询
   * queryKey 包含所有筛选条件，任一变化都会重新查询
   * enabled: searchActive 控制是否执行搜索（首次需 URL 有参数才激活）
   */
  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    error,
    refetch,
  } = useInfiniteQuery({
    queryKey: ['search', query, dateFrom, dateTo, sceneType, cameraModel, selectedTags],
    queryFn: ({ pageParam = 1 }) =>
      searchPhotos({
        query: query || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        tags: selectedTags.length > 0 ? selectedTags : undefined,
        scene_type: sceneType || undefined,
        camera_model: cameraModel || undefined,
        page: pageParam,
        page_size: PAGE_SIZE,
      }),
    getNextPageParam: (lastPage) => {
      const totalPages = Math.ceil(lastPage.total / lastPage.page_size);
      return lastPage.page < totalPages ? lastPage.page + 1 : undefined;
    },
    initialPageParam: 1,
    enabled: searchActive,
  });

  /**
   * 防抖自动搜索 effect：
   * - 首次挂载时：如果 URL 有参数则自动激活搜索
   * - 后续变化：300ms 防抖后自动同步 URL 并触发搜索
   * - 防抖避免用户快速输入时频繁请求
   */
  useEffect(() => {
    if (initialMount.current) {
      initialMount.current = false;
      if (searchParams.toString()) {
        setSearchActive(true);
        refetch();
      }
      return;
    }

    // 300ms 防抖：延迟同步 URL 并触发搜索
    const timer = setTimeout(() => {
      setSearchParams(buildParams(), { replace: true });
      if (!searchActive) setSearchActive(true);
      refetch();
    }, 300);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, dateFrom, dateTo, sceneType, cameraModel, selectedTags]);

  /**
   * 无限滚动实现：
   * IntersectionObserver 监听哨兵元素，
   * 当哨兵进入视口且有下一页时触发加载。
   */
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { threshold: 0.1 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  /**
   * 表单提交处理：同步 URL 参数并触发搜索
   * 阻止默认表单提交行为，手动控制搜索流程
   */
  const handleSearch = (e?: FormEvent) => {
    e?.preventDefault();
    setSearchParams(buildParams(), { replace: true });
    setSearchActive(true);
    refetch();
  };

  /**
   * 将当前筛选状态构建为 URL 查询参数
   * 仅包含有值的参数，空值不写入 URL
   */
  const buildParams = (): URLSearchParams => {
    const p = new URLSearchParams();
    if (query) p.set('query', query);
    if (dateFrom) p.set('date_from', dateFrom);
    if (dateTo) p.set('date_to', dateTo);
    if (sceneType) p.set('scene_type', sceneType);
    if (cameraModel) p.set('camera_model', cameraModel);
    if (selectedTags.length > 0) p.set('tags', selectedTags.join(','));
    return p;
  };

  /**
   * 切换标签选中状态
   * 已选中则移除，未选中则添加
   */
  const toggleTag = (tag: string) => {
    setSelectedTags((prev) =>
      prev.includes(tag) ? prev.filter((t) => t !== tag) : [...prev, tag],
    );
  };

  /**
   * 重置所有筛选条件到初始状态
   * 清空所有 useState 和 URL 参数，关闭搜索结果
   */
  const resetFilters = () => {
    setDateFrom('');
    setDateTo('');
    setSceneType('');
    setCameraModel('');
    setSelectedTags([]);
    setQuery('');
    setSearchParams({}, { replace: true });
    setSearchActive(false);
  };

  // 计算当前活跃的筛选条件数量（用于显示角标）
  const activeFilterCount =
    (dateFrom ? 1 : 0) +
    (dateTo ? 1 : 0) +
    (sceneType ? 1 : 0) +
    (cameraModel ? 1 : 0) +
    selectedTags.length;

  // 合并所有分页的搜索结果为一个数组
  const allPhotos = data?.pages.flatMap((page) => page.hits) ?? [];
  // 获取搜索结果总数和总页数（从第一页数据中提取）
  const total = data?.pages[0]?.total ?? 0;
  const totalPages = data?.pages[0]?.total_pages ?? 0;

  return (
    <div className="space-y-6">
      {/* 搜索输入框 + 提交按钮 */}
      <form onSubmit={handleSearch} className="flex gap-2">
        <div className="relative flex-1">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
          <input
            type="text"
            value={query}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setQuery(e.target.value)}
            placeholder="输入关键词搜索照片…"
            className="w-full pl-9 pr-4 py-2.5 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white placeholder-gray-400"
          />
        </div>
        <button
          type="submit"
          disabled={isLoading}
          className="px-5 py-2.5 text-sm font-medium text-white bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg transition-colors"
        >
          搜索
        </button>
      </form>

      {/* 筛选面板切换按钮 */}
      <div>
        <button
          type="button"
          onClick={() => setShowFilters((v) => !v)}
          className={`inline-flex items-center gap-1.5 text-sm font-medium transition-colors ${
            showFilters ? 'text-purple-600' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <svg
            className={`w-4 h-4 transition-transform ${showFilters ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M19 9l-7 7-7-7"
            />
          </svg>
          筛选条件
          {activeFilterCount > 0 && (
            <span className="inline-flex items-center justify-center w-5 h-5 text-[11px] font-semibold text-white bg-purple-500 rounded-full">
              {activeFilterCount}
            </span>
          )}
        </button>
      </div>

      {/* 筛选面板：日期范围、场景类型、相机型号、标签多选 */}
      {showFilters && (
        <div className="p-5 bg-white border border-gray-200 rounded-xl space-y-5">
          {/* 日期范围筛选：起始日期 → 结束日期 */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">
               拍摄日期
            </label>
            <div className="flex items-center gap-2">
              <input
                type="date"
                value={dateFrom}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setDateFrom(e.target.value)}
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
              <span className="text-xs text-gray-400">至</span>
              <input
                type="date"
                value={dateTo}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setDateTo(e.target.value)}
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
              />
            </div>
          </div>

          {/* 场景类型下拉筛选 */}
          <div>
            <label htmlFor="filter-scene" className="block text-xs font-medium text-gray-500 mb-2">
               场景类型
            </label>
            <select
              id="filter-scene"
              value={sceneType}
              onChange={(e: ChangeEvent<HTMLSelectElement>) => setSceneType(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.scene_types || []).map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </div>

          {/* 相机型号下拉筛选 */}
          <div>
            <label htmlFor="filter-camera" className="block text-xs font-medium text-gray-500 mb-2">
               相机型号
            </label>
            <select
              id="filter-camera"
              value={cameraModel}
              onChange={(e: ChangeEvent<HTMLSelectElement>) => setCameraModel(e.target.value)}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent bg-white"
            >
              <option value="">全部</option>
              {(filters?.cameras || []).map((c) => (
                <option key={c} value={c}>{c}</option>
              ))}
            </select>
          </div>

          {/* 标签多选：从 filters 接口获取可选标签列表 */}
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-2">标签</label>
            {filters?.tags && filters.tags.length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {filters.tags.map((tag) => {
                  const active = selectedTags.includes(tag);
                  return (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => toggleTag(tag)}
                      className={`text-xs px-2.5 py-1 rounded-full font-medium transition-colors ${
                        active
                          ? 'bg-purple-100 text-purple-700 border border-purple-300'
                          : 'bg-gray-100 text-gray-600 border border-transparent hover:bg-gray-200'
                      }`}
                    >
                      {tag}
                    </button>
                  );
                })}
              </div>
            ) : (
              <p className="text-xs text-gray-400">暂无可用标签</p>
            )}
          </div>

          {/* 重置按钮：有激活的筛选条件时显示 */}
          {activeFilterCount > 0 && (
            <div className="pt-1">
              <button
                type="button"
                onClick={resetFilters}
                className="text-xs text-gray-500 hover:text-gray-700 underline"
              >
                重置所有筛选条件
              </button>
            </div>
          )}
        </div>
      )}

      {/* 初始加载状态：显示旋转加载器 */}
      {isLoading && <Spinner />}

      {/* 错误状态：显示错误信息 + 重试按钮 */}
      {isError && (
        <div className="text-center py-12">
          <p className="text-sm text-red-500">{(error as Error)?.message || '搜索失败'}</p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 text-sm text-purple-600 hover:text-purple-700 font-medium"
          >
            重试
          </button>
        </div>
      )}

      {/* 空结果：搜索已完成但未找到匹配照片 */}
      {!isLoading && !isError && allPhotos.length === 0 && data && (
        <div className="text-center py-16">
          <svg
            className="w-12 h-12 mx-auto text-gray-300 mb-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
          <p className="text-sm text-gray-500">未找到匹配的照片，试试其他关键词</p>
        </div>
      )}

      {/* 搜索结果展示 */}
      {allPhotos.length > 0 && (
        <>
          {/* 结果统计：总数 + 分页信息 */}
          <p className="text-xs text-gray-400">
            共找到 <span className="font-medium text-gray-600">{total}</span> 张照片
            {totalPages > 1 && <>，第 {data?.pages.length}/{totalPages} 页</>}
          </p>

          {/* 响应式照片网格 */}
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {allPhotos.map((photo) => (
              <PhotoCard key={photo.path} photo={photo} />
            ))}
          </div>

          {/* 无限滚动哨兵 */}
          <div ref={sentinelRef} className="h-4" />

          {/* 加载更多时的骨架屏 */}
          {isFetchingNextPage && (
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
              {Array.from({ length: 4 }).map((_, i) => (
                <SkeletonCard key={`loading-${i}`} />
              ))}
            </div>
          )}

          {/* 全部加载完成提示 */}
          {!hasNextPage && (
            <p className="py-8 text-center text-sm text-gray-400">已加载全部照片</p>
          )}
        </>
      )}
    </div>
  );
}
