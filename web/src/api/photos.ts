import apiClient from './client';
import type {
  PhotoDocument,
  SearchRequest,
  SearchResponse,
  StatsResponse,
  FiltersResponse,
} from '../types';

export interface FetchPhotosParams {
  date_from?: string;
  date_to?: string;
  status?: string;
  page?: number;
  page_size?: number;
}

export interface FetchPhotosResponse {
  photos: PhotoDocument[];
  total: number;
  page: number;
  page_size: number;
}

export async function fetchPhotos(
  params: FetchPhotosParams,
): Promise<FetchPhotosResponse> {
  const { data } = await apiClient.get<{ hits: PhotoDocument[]; total: number; page: number; page_size: number; total_pages: number }>('/photos', { params });
  return {
    photos: data.hits ?? [],
    total: data.total,
    page: data.page,
    page_size: data.page_size,
  };
}

export async function searchPhotos(req: SearchRequest): Promise<SearchResponse> {
  const { data } = await apiClient.post<SearchResponse>('/search', req);
  return data;
}

export async function fetchPhotoDetail(id: string): Promise<PhotoDocument> {
  const { data } = await apiClient.get<PhotoDocument>(`/photos/${id}`);
  return data;
}

export async function fetchStats(): Promise<StatsResponse> {
  const { data } = await apiClient.get<StatsResponse>('/stats');
  return data;
}

export async function fetchFilters(): Promise<FiltersResponse> {
  const { data } = await apiClient.get<FiltersResponse>('/filters');
  return data;
}
