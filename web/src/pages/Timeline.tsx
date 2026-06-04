/**
 * 时间线页面（Timeline）
 *
 * 页面定位：
 * 照片库的主页，按拍摄日期分组展示所有已导入的照片，
 * 支持无限滚动加载和状态过滤筛选。
 *
 * 功能特性：
 * - 按日期分组展示全部照片（useMemo 缓存分组结果）
 * - 无限滚动加载（IntersectionObserver 监听哨兵元素）
 * - 加载状态显示骨架屏（SkeletonCard，脉冲动画）
 * - 错误状态与空状态处理（错误提示 / 引导用户导入照片）
 * - 分析中照片显示脉冲动画占位（animate-pulse）
 *
 * 数据流：
 * useInfiniteQuery 分页获取 → pages.flatMap 合并所有页数据
 * → extractDate 提取拍摄日期 → Map 分组 → 日期降序排列
 * → 渲染日期分组网格
 *
 * 交互逻辑：
 * - 点击照片卡片 → navigate 跳转到 /photo/{path} 详情页
 * - 滚动至底部哨兵元素 → IntersectionObserver 触发 fetchNextPage
 * - 加载下一页时保存并恢复滚动位置（避免页面跳动）
 *
 * 组件结构：
 * - SkeletonCard：加载中的骨架占位卡片（脉冲动画）
 * - PhotoCard：单张照片展示卡片（缩略图 + 描述 + 标签 + 状态标签）
 * - Timeline：主页面组件，管理无限滚动和分组逻辑
 */
import { useInfiniteQuery } from '@tanstack/react-query';
import { useNavigate, Link } from 'react-router-dom';
import { useRef, useEffect, useMemo } from 'react';
import { fetchPhotos } from '../api/photos';
import type { PhotoDocument } from '../types';

/**
 * 标签颜色调色板（8 色）
 * 复用自 PhotoDetail 组件，通过 DJB2 哈希确保同一标签始终同色
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
 * DJB2 哈希 → 为标签分配确定性颜色
 */
function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  return TAG_COLORS[Math.abs(hash) % TAG_COLORS.length];
}

/** 地点展示层级 */
type LocationHierarchyLevel = 'cityDistrict' | 'provinceCity' | 'countryProvince';

/** 地点聚合结果 */
interface LocationSummary {
  level: LocationHierarchyLevel;
  locations: { label: string; count: number; city?: string; district?: string; province?: string; country?: string }[];
}

/** 标签聚合结果 */
interface TagSummaryItem {
  tag: string;
  count: number;
}

/**
 * 根据日期组内所有位置数据的地理分布，决定展示层级
 */
function determineHierarchyLevel(
  uniqueCities: Set<string>,
  uniqueCountries: Set<string>,
): LocationHierarchyLevel {
  if (uniqueCities.size <= 1) return 'cityDistrict';
  if (uniqueCountries.size > 1) return 'countryProvince';
  return 'provinceCity';
}

/**
 * 计算日期组的地点聚合摘要
 * - 按 (city, district) 组合统计出现频次
 * - 直辖市（city 为空）时 fallback 到 province 作为 effectiveCity
 * - 根据所有唯一城市/国家的分布决定展示层级
 * - 取频次最高的前 2 个地点
 */
function computeLocationSummary(photos: PhotoDocument[]): LocationSummary | null {
  const freq = new Map<string, number>();
  const uniqueCountries = new Set<string>();
  const uniqueCities = new Set<string>();

  for (const p of photos) {
    // 直辖市场景下高德 API 的 city 字段为空，fallback 到 province
    const effectiveCity = p.city || p.province || '';
    const district = p.district || '';
    if (!effectiveCity && !district) continue;
    const key = `${effectiveCity}|${district}`;
    freq.set(key, (freq.get(key) ?? 0) + 1);
    if (effectiveCity) uniqueCities.add(effectiveCity);
    if (p.country) uniqueCountries.add(p.country);
  }

  if (freq.size === 0) return null;

  const level = determineHierarchyLevel(uniqueCities, uniqueCountries);

  const top = [...freq.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 2);

  const findPhotoByKey = (cityPart: string, districtPart: string): PhotoDocument | undefined =>
    photos.find((p) =>
      (p.city || p.province || '') === cityPart && (p.district || '') === districtPart,
    );

  const locations = top.map(([key]) => {
    const [cityPart, districtPart] = key.split('|');
    const sample = findPhotoByKey(cityPart, districtPart);
    switch (level) {
      case 'cityDistrict':
        return {
          label: districtPart ? `${cityPart}·${districtPart}` : cityPart,
          count: freq.get(key)!,
        };
      case 'provinceCity': {
        // 直辖市 fallback 时 province === cityPart，避免重复显示
        const prov = sample?.province;
        const label = !prov || prov === cityPart ? cityPart : `${prov}·${cityPart}`;
        return { label, count: freq.get(key)! };
      }
      case 'countryProvince': {
        // 用 Set 去重，避免 "中国·北京市·北京市" 这种重复
        const parts = [sample?.country, sample?.province, cityPart].filter(Boolean);
        return { label: [...new Set(parts)].join('·'), count: freq.get(key)! };
      }
    }
  });

  return { level, locations };
}

/**
 * 计算日期组的标签聚合摘要
 * - 统计所有标签出现频次
 * - 取频次最高的前 3 个标签
 */
function computeTagSummary(photos: PhotoDocument[]): TagSummaryItem[] {
  const freq = new Map<string, number>();
  for (const p of photos) {
    if (!p.tags) continue;
    for (const tag of p.tags) {
      freq.set(tag, (freq.get(tag) ?? 0) + 1);
    }
  }
  return [...freq.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([tag, count]) => ({ tag, count }));
}

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
 * 从照片文档中提取日期字符串
 *
 * 优先级：EXIF 拍摄时间（date_time_original）> 文件修改时间（mtime）
 * 返回格式：YYYY-MM-DD，兜底返回 '未知日期'
 * mtime 自动检测时间戳单位：大于 1e12 视为毫秒级，否则视为秒级
 *
 * @param photo - 照片文档对象（包含 EXIF 元数据和文件 mtime）
 * @returns 格式化的日期字符串（YYYY-MM-DD），无法解析时返回 '未知日期'
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
 * 拆分 YYYY-MM-DD 格式字符串，重组为中文日期表达。
 * 月/日部分去掉前导零（parseInt 后自动处理）。
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
 * 构建照片 URL 路径
 *
 * 拼接 /photos/ 前缀，并去除 path 中可能的前导斜杠以防止路径错误
 *
 * @param path - 照片文件中继路径
 * @returns 完整的照片 URL 路径
 */
function photoSrc(path: string): string {
  return `/photos/${path.replace(/^\/+/, '')}`;
}

/**
 * 骨架屏占位卡片组件
 *
 * 在数据加载期间显示脉冲动画的灰色占位块，
 * 模拟真实卡片的布局（正方形图片 + 两行文字）。
 * 使用 Tailwind animate-pulse 实现呼吸闪烁效果。
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
 * - 显示照片缩略图（带懒加载 loading="lazy"）
 * - 图片加载失败时隐藏 img 标签并显示占位文字
 * - 分析中的照片显示右上角状态标签 + 骨架屏描述占位
 * - 已分析的照片显示 AI 生成的描述和标签（最多 3 个 + 溢出计数）
 * - 点击跳转到照片详情页（/photo/{id}）
 *
 * @param photo - 照片文档对象，包含路径、描述、标签、状态等信息
 */
function PhotoCard({ photo }: { photo: PhotoDocument }) {
  const navigate = useNavigate();

  return (
    <button
      type="button"
      onClick={() => navigate(`/photo/${photo.id}`)}
      className="group cursor-pointer text-left"
    >
      {/* 缩略图区域：方形裁剪，悬停时有缩放动画 */}
      <div className="aspect-square overflow-hidden rounded-lg bg-gray-100 relative">
        {/*
         * 照片缩略图，使用 ?thumb=1 参数请求后端缩略图服务
         * loading="lazy" 启用浏览器原生懒加载
         * onError 处理图片加载失败：隐藏 img 标签，显示占位文字
         */}
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
        {/*
         * 图片加载失败时的占位文字
         * 初始状态 hidden，onError 触发后显示
         */}
        <div className="hidden absolute inset-0 flex items-center justify-center bg-gray-100 text-gray-400 text-sm">
          无法加载图片
        </div>
        {/* 分析中状态：右上角显示"分析中"黄色标签 */}
        {photo.status === 'analyzing' && (
          <span className={`absolute top-2 right-2 text-[10px] px-1.5 py-0.5 rounded font-medium ${STATUS_COLORS.analyzing}`}>
            {STATUS_LABELS.analyzing}
          </span>
        )}
      </div>

      <div className="mt-2 space-y-1.5">
        {/*
         * 分析中 → 显示骨架屏占位（模拟文字 + 标签形状）
         * 已分析  → 显示 AI 生成的真实描述和标签
         */}
        {photo.status === 'analyzing' ? (
          <>
            {/* 分析中：骨架屏模拟两行文字描述 */}
            <div className="animate-pulse space-y-1.5">
              <div className="h-3 w-full rounded bg-gray-200" />
              <div className="h-3 w-3/4 rounded bg-gray-200" />
            </div>
            {/* 分析中：骨架屏模拟三个标签 */}
            <div className="animate-pulse flex flex-wrap gap-1">
              <div className="h-5 w-10 rounded bg-gray-200" />
              <div className="h-5 w-12 rounded bg-gray-200" />
              <div className="h-5 w-8 rounded bg-gray-200" />
            </div>
          </>
        ) : (
          <>
            {/* AI 生成的照片描述，line-clamp-2 限制最多显示 2 行 */}
            {photo.description && (
              <p className="line-clamp-2 text-sm leading-snug text-gray-700">
                {photo.description}
              </p>
            )}
            {/* 标签列表：最多显示 3 个标签，超出部分显示 +N 计数 */}
            {photo.tags && photo.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {photo.tags.slice(0, 3).map((tag) => (
                  <span
                    key={tag}
                    className="inline-block rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-700"
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
 * - 四种渲染状态：加载中（骨架屏）、错误、空数据、正常内容
 *
 * 数据流：
 * useInfiniteQuery(pages) → flatMap 合并所有页 → extractDate 分组 → 渲染
 */
export default function Timeline() {
  /** 无限滚动哨兵元素引用，IntersectionObserver 监听此元素是否进入视口 */
  const sentinelRef = useRef<HTMLDivElement>(null);
  /** 记录加载下一页前的滚动位置，用于新数据到达后恢复，防止页面跳动 */
  const scrollPositionRef = useRef<number>(0);

  /**
   * useInfiniteQuery 配置说明：
   * - queryKey: ['photos'] — React Query 缓存键，用于缓存管理和自动失效
   * - queryFn: 调用 fetchPhotos API，默认 pageParam=1，每页 50 条
   * - getNextPageParam: 根据 total/page_size 计算总页数，判断是否还有下一页
   * - initialPageParam: 起始页码为 1
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
   * 无限滚动副作用
   *
   * 使用 IntersectionObserver 监听哨兵元素（sentinelRef），
   * 当哨兵元素进入视口（threshold=0.1，即 10% 可见）且满足条件时：
   * 1. 有下一页数据（hasNextPage）
   * 2. 当前未在加载中（!isFetchingNextPage）
   * 则记录当前滚动位置并触发 fetchNextPage 加载更多数据。
   * 组件卸载时自动断开 observer 连接，防止内存泄漏。
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
   * 滚动位置恢复副作用
   *
   * 当新数据加载完成（isFetchingNextPage 变为 false 且 data 更新）时，
   * 如果之前保存了滚动位置，则恢复到该位置并清零记录。
   * 避免因新数据插入导致视口内容跳跃。
   */
  useEffect(() => {
    if (scrollPositionRef.current > 0 && !isFetchingNextPage) {
      window.scrollTo(0, scrollPositionRef.current);
      scrollPositionRef.current = 0;
    }
  }, [data, isFetchingNextPage]);

  /**
   * 按日期分组照片（useMemo 缓存，仅在 data 变化时重新计算）
   *
   * 处理步骤：
   * 1. 将所有分页数据（pages）的照片数组扁平化合并
   * 2. 遍历每张照片，使用 extractDate 提取日期字符串
   * 3. 按日期分组存入 Map<string, PhotoDocument[]>
   * 4. 转换为数组并按日期降序排列（最新的日期在前）
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

    return [...groups.entries()]
      .sort(([a], [b]) => b.localeCompare(a))
      .map(([dateStr, photos]) => ({
        dateStr,
        photos,
        tagSummary: computeTagSummary(photos),
        locationSummary: computeLocationSummary(photos),
      }));
  }, [data]);

  // ========== 条件渲染：错误状态 ==========
  // 请求失败时显示警告图标和具体错误信息
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

  // ========== 条件渲染：初始加载中 ==========
  // 首屏数据尚未加载完成，显示 6 个骨架屏卡片占位
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  // ========== 条件渲染：空数据 ==========
  // 数据加载完成但没有照片时，显示空状态引导提示
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

  // ========== 主渲染：按日期分组的照片内容 ==========
  return (
    <div className="space-y-8">
      {/*
       * 按日期分组迭代渲染：
       * 每组包含一个 sticky 定位的日期标题（带毛玻璃效果）
       * 和一个响应式照片网格（2/3/4 列自适应）
       */}
      {groupedPhotos.map(({ dateStr, photos, tagSummary, locationSummary }) => (
        <section key={dateStr}>
          {/*
           * 日期分组标题
           * sticky top-0：滚动时固定在顶部
           * backdrop-blur-sm：半透明毛玻璃效果
           * z-10：确保标题在照片卡片之上
           */}
          <h2 className="sticky top-0 z-10 mb-4 bg-gray-50/90 px-2 py-2 backdrop-blur-sm">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <span className="text-lg font-semibold text-gray-800">{formatDateLabel(dateStr)}</span>
              {locationSummary?.locations.map((loc) => {
                const locParams = new URLSearchParams();
                if (loc.city) locParams.set('city', loc.city);
                if (loc.district) locParams.set('district', loc.district);
                if (loc.province) locParams.set('province', loc.province);
                if (loc.country) locParams.set('country', loc.country);
                return (
                  <Link
                    key={loc.label}
                    to={`/search?${locParams.toString()}`}
                    className="inline-flex items-center gap-0.5 rounded bg-indigo-50 px-1.5 py-0.5 text-xs font-normal text-indigo-600 hover:bg-indigo-100 cursor-pointer transition-colors"
                  >
                    <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M17.657 16.657L13.414 20.9a1.998 1.998 0 01-2.827 0l-4.244-4.243a8 8 0 1111.314 0z" />
                      <path strokeLinecap="round" strokeLinejoin="round" d="M15 11a3 3 0 11-6 0 3 3 0 016 0z" />
                    </svg>
                    {loc.label}
                  </Link>
                );
              })}
              {tagSummary.map(({ tag }) => (
                <Link
                  key={tag}
                  to={`/search?tags=${encodeURIComponent(tag)}`}
                  className={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium hover:opacity-80 cursor-pointer transition-opacity ${tagColor(tag)}`}
                >
                  {tag}
                </Link>
              ))}
            </div>
          </h2>
          {/*
           * 响应式照片网格：
           * - 移动端：2 列
           * - md 断点（768px）：3 列
           * - lg 断点（1024px）：4 列
           * gap-4 提供统一的卡片间距
           */}
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
            {photos.map((photo) => (
              <PhotoCard key={photo.id} photo={photo} />
            ))}
          </div>
        </section>
      ))}

      {/*
       * 无限滚动哨兵元素
       * IntersectionObserver 监听此元素进入视口，
       * 触发 fetchNextPage 加载更多数据。
       * h-4 确保有足够的高度被 observer 检测到。
       */}
      <div ref={sentinelRef} className="h-4" />

      {/*
       * 加载下一页时的骨架屏占位
       * isFetchingNextPage 为 true 时显示 4 个骨架屏卡片，
       * 提示用户数据正在加载中。
       */}
      {isFetchingNextPage && (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <SkeletonCard key={`loading-${i}`} />
          ))}
        </div>
      )}

      {/*
       * 全部加载完成提示
       * hasNextPage 为 false 且存在数据时，在底部显示
       * "已加载全部照片" 的结束提示文字。
       */}
      {!hasNextPage && groupedPhotos.length > 0 && (
        <p className="py-8 text-center text-sm text-gray-400">已加载全部照片</p>
      )}
    </div>
  );
}
