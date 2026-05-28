export interface Photo {
  id: string;
  path: string;
  mtime: number;
  size: number;
  status: 'unanalyzed' | 'analyzing' | 'analyzed' | 'failed' | 'pending_analysis';
  analyzed_at?: number;
  created_at: number;
}

export interface EXIFInfo {
  date_time_original?: string;
  camera_model?: string;
  lens_model?: string;
  focal_length?: string;
  aperture?: string;
  iso?: number;
  gps_lat?: number;
  gps_lon?: number;
}

export interface AnalysisResult {
  description: string;
  tags: string[];
  objects: string[];
  scene_type: string;
  colors: string[];
  people_count: number;
  has_text: boolean;
  confidence?: number;
}

export interface PhotoDocument extends Photo, AnalysisResult {
  exif?: EXIFInfo;
}

export interface SearchRequest {
  query?: string;
  date_from?: string;
  date_to?: string;
  tags?: string[];
  objects?: string[];
  scene_type?: string;
  camera_model?: string;
  page: number;
  page_size: number;
}

export interface SearchResponse {
  hits: PhotoDocument[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface StatsResponse {
  total: number;
  by_status: Record<string, number>;
  recent_count: number;
}

export interface FiltersResponse {
  tags: string[];
  scene_types: string[];
  cameras: string[];
}
