/**
 * @file 照片相关 API 调用函数模块
 *
 * 封装所有与照片资源交互的 HTTP 请求，包括：
 * - 时间线列表查询（fetchPhotos）
 * - 全文搜索（searchPhotos）
 * - 单张照片详情（fetchPhotoDetail）
 * - 统计信息（fetchStats）
 * - 筛选选项（fetchFilters）
 *
 * 所有函数通过统一的 apiClient 实例发起请求，
 * 自动携带 baseURL、超时配置和错误拦截器。
 */

import apiClient from './client';
import type {
  PhotoDocument,
  SearchRequest,
  SearchResponse,
  StatsResponse,
  FiltersResponse,
} from '../types';

/**
 * 照片列表查询参数
 *
 * 用于 GET /api/photos 端点，支持按日期范围、处理状态筛选，
 * 以及分页控制。所有字段均为可选，未传则使用服务端默认值。
 */
export interface FetchPhotosParams {
  /** 起始日期，格式 YYYY-MM-DD，仅返回此日期及之后的照片 */
  date_from?: string;
  /** 结束日期，格式 YYYY-MM-DD，仅返回此日期及之前的照片 */
  date_to?: string;
  /** 按处理状态过滤，可选值：unanalyzed | analyzing | analyzed | failed | pending_analysis */
  status?: string;
  /** 页码，从 1 开始，默认 1 */
  page?: number;
  /** 每页数量，默认 50，最大 100 */
  page_size?: number;
}

/**
 * 照片列表查询响应
 *
 * 与服务端实际返回的字段略有差异：
 * 服务端返回 hits（命中列表），此处映射为 photos（更语义化）。
 */
export interface FetchPhotosResponse {
  /** 照片文档列表 */
  photos: PhotoDocument[];
  /** 符合条件的照片总数 */
  total: number;
  /** 当前页码 */
  page: number;
  /** 每页数量 */
  page_size: number;
}

/**
 * 获取照片时间线列表
 *
 * 调用 GET /api/photos 端点，根据筛选参数获取分页照片列表。
 * 将服务端返回的 hits 字段映射为 photos，确保前端类型一致性。
 *
 * @param params - 查询参数（日期范围、状态、分页）
 * @returns Promise<FetchPhotosResponse> 包含照片列表、总数和分页信息
 */
export async function fetchPhotos(
  params: FetchPhotosParams,
): Promise<FetchPhotosResponse> {
  // 使用 axios 泛型声明响应体类型，确保类型安全
  const { data } = await apiClient.get<{ hits: PhotoDocument[]; total: number; page: number; page_size: number; total_pages: number }>('/photos', { params });
  return {
    photos: data.hits ?? [],  // hits 为空时兜底为空数组，避免前端遍历异常
    total: data.total,
    page: data.page,
    page_size: data.page_size,
  };
}

/**
 * 全文搜索照片
 *
 * 调用 POST /api/search 端点，支持关键词搜索、日期范围、标签、
 * 物体、场景类型、相机型号等多维度筛选。
 *
 * @param req - 搜索请求体（SearchRequest），包含查询关键词和各种过滤条件
 * @returns Promise<SearchResponse> 搜索结果，包含命中列表、总数和分页信息
 */
export async function searchPhotos(req: SearchRequest): Promise<SearchResponse> {
  const { data } = await apiClient.post<SearchResponse>('/search', req);
  return data;
}

/**
 * 获取单张照片详情
 *
 * 调用 GET /api/photos/{id} 端点，id 为照片文件路径的 SHA-256 哈希值。
 * 返回完整的照片文档，包括 EXIF 元数据和 AI 分析结果。
 *
 * @param id - 照片 ID（文件路径的 SHA-256 哈希值）
 * @returns Promise<PhotoDocument> 完整的照片文档信息
 */
export async function fetchPhotoDetail(id: string): Promise<PhotoDocument> {
  // 对 ID 进行 URL 编码，防止特殊字符导致请求路径异常
  const { data } = await apiClient.get<PhotoDocument>(`/photos/${encodeURIComponent(id)}`);
  return data;
}

/**
 * 获取照片统计信息
 *
 * 调用 GET /api/stats 端点，返回照片总数及各处理状态的数量分布，
 * 用于前端仪表盘展示。
 *
 * @returns Promise<StatsResponse> 统计数据，包含 total、by_status、recent_count
 */
export async function fetchStats(): Promise<StatsResponse> {
  const { data } = await apiClient.get<StatsResponse>('/stats');
  return data;
}

/**
 * 获取搜索筛选选项
 *
  * 调用 GET /api/filters 端点，返回所有可用的标签、场景类型、地理信息和状态列表，
  * 用于前端搜索页面的下拉筛选项。
  *
  * @returns Promise<FiltersResponse> 筛选选项，包含 tags、scene_types、countries、provinces、cities、districts、statuses
 */
export async function fetchFilters(): Promise<FiltersResponse> {
  const { data } = await apiClient.get<FiltersResponse>('/filters');
  return data;
}
