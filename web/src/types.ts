/**
 * 前端类型定义文件
 *
 * 定义了应用中所有 TypeScript 接口和类型，
 * 与后端 API 返回的 JSON 数据结构一一对应。
 * 包含照片元数据、EXIF 信息、AI 分析结果、
 * 地理位置、搜索请求/响应等核心数据模型。
 */

/**
 * 照片基础信息接口
 *
 * 描述一张照片的基本元数据，对应后端 ES 中的文档结构。
 * 包含文件标识、路径、修改时间、大小和处理状态。
 */
/**
 * 前端 TypeScript 类型定义
 *
 * 本文件定义了前端应用中所有的数据类型接口，对应后端 REST API 的 JSON 响应结构。
 * 这些类型确保了前后端数据交互的类型安全，避免运行时数据格式不匹配的问题。
 *
 * 数据流关系：
 *   Photo（基础照片）→ PhotoDocument（完整照片文档，合并了照片信息 + AI 分析结果 + 地理位置）
 *   SearchRequest（搜索请求）→ SearchResponse（搜索结果，包含 PhotoDocument 列表）
 *   StatsResponse（统计信息）/ FiltersResponse（筛选选项）为独立查询接口
 */

/**
 * 照片基础信息接口
 *
 * 对应后端存储的照片核心元数据，包含文件系统级别的基本信息。
 * 每张照片通过其文件路径的 SHA-256 哈希值作为唯一标识符。
 *
 * @property id - 照片唯一标识符（文件路径的 SHA-256 哈希值）
 * @property path - 相对于 photo_base_path 的文件路径
 * @property mtime - 文件最后修改时间的 Unix 时间戳（秒）
 * @property size - 文件大小（字节）
 * @property status - 照片处理状态
 * @property analyzed_at - AI 分析完成时间的 Unix 时间戳（秒），仅分析完成后存在
 * @property created_at - 照片首次被监控发现的时间戳（秒）
 */
/**
 * 前端 TypeScript 类型定义文件
 *
 * 定义与后端 API 交互所需的所有数据结构。
 * 类型与后端 internal/types/types.go 保持一致，
 * 确保前后端数据契约的类型安全。
 */

/**
 * 照片基础信息接口
 *
 * 包含照片的元数据信息，对应后端 API 返回的 PhotoDocument 基础字段。
 * 每张照片由文件路径的 SHA-256 哈希作为唯一标识。
 */
export interface Photo {
  /** 照片唯一标识符（文件路径的 SHA-256 哈希） */
  id: string;
  /** 照片文件的相对路径（相对于 photo_base_path） */
  path: string;
  /** 文件最后修改时间（Unix 时间戳，秒） */
  mtime: number;
  /** 文件大小（字节） */
  size: number;
  /** 处理状态：unanalyzed（未分析）/ analyzing（分析中）/ analyzed（已分析）/ failed（失败）/ pending_analysis（等待重试） */
  status: 'unanalyzed' | 'analyzing' | 'analyzed' | 'failed' | 'pending_analysis';
  /** 分析完成时间（Unix 时间戳，秒），仅 analyzed 状态有值 */
  analyzed_at?: number;
  /** 照片拍摄/创建时间（Unix 时间戳，秒） */
  created_at: number;
}

/**
 * EXIF 元数据信息接口
 *
 * 从照片文件中提取的 EXIF 信息，包含相机参数和 GPS 坐标。
 * 并非所有照片都包含完整的 EXIF 数据，字段均为可选。
 */
export interface EXIFInfo {
  /** 拍摄时间（ISO 8601 格式，如 "2024-01-01T10:30:00Z"） */
  date_time_original?: string;
  /** 相机型号（如 "iPhone 15 Pro"、"Canon EOS R5"） */
  camera_model?: string;
  /** 镜头型号 */
  lens_model?: string;
  /** 焦距（如 "6.9mm"） */
  focal_length?: string;
  /** 光圈值（如 "f/1.8"） */
  aperture?: string;
  /** 快门速度（如 "1/250"） */
  shutter_speed?: string;
  /** ISO 感光度 */
  iso?: number;
  /** GPS 纬度（十进制度数） */
  gps_lat?: number;
  /** GPS 经度（十进制度数） */
  gps_lon?: number;
}

/**
 * 颜色信息接口
 *
 * AI 分析结果中提取的主要颜色，用于前端展示颜色标签。
 */
export interface ColorInfo {
  /** 颜色中文名称（如 "绿色"、"蓝色"） */
  name: string;
  /** 颜色十六进制值（如 "#228B22"），用于前端背景色渲染 */
  hex: string;
}

/**
 * AI 分析结果接口
 *
 * 多模态 LLM（Ollama/OpenAI）对照片内容的结构化分析输出。
 * 包含描述、标签、物体检测、场景分类、颜色等信息。
 */
export interface AnalysisResult {
  /** 照片内容的自然语言描述 */
  description: string;
  /** 语义标签列表（如 ["公园", "野餐", "阳光"]） */
  tags: string[];
  /** 检测到的物体列表（如 ["人", "草地", "树木"]） */
  objects: string[];
  /** 场景类型：indoor（室内）/ outdoor（室外）/ unknown（未知） */
  scene_type: string;
  /** 主要颜色信息列表（含名称和十六进制值） */
  colors: ColorInfo[];
  /** 检测到的人数 */
  people_count: number;
  /** 照片中是否包含文字 */
  has_text: boolean;
  /** 照片中识别到的文字内容（has_text 为 true 时有值） */
  text: string;
  /** AI 分析置信度（0~1），值越高表示分析结果越可靠 */
  confidence?: number;
}

/**
 * 地理位置信息接口
 *
 * 通过逆地理编码（高德 API）将 GPS 坐标转换为可读的地址信息。
 * 存储在 Elasticsearch 中，支持按地理位置筛选。
 */
export interface GeoInfo {
  /** 国家名称 */
  country?: string;
  /** 省/州名称 */
  province?: string;
  /** 城市名称 */
  city?: string;
  /** 区/县名称 */
  district?: string;
  /** 乡镇/街道名称 */
  township?: string;
  /** 商圈名称 */
  business_area?: string;
  /** 街道名称 */
  street?: string;
  /** 门牌号 */
  street_number?: string;
  /** 详细地址 */
  address?: string;
  /** 格式化的完整地址（高德 API 返回的 formatted_address） */
  formatted_address?: string;
}

/**
 * 照片完整文档接口（继承 Photo + AnalysisResult）
 *
 * 前端渲染照片详情时使用的完整数据结构。
 * 合并了基础信息、AI 分析结果、EXIF 元数据和地理位置信息。
 */
export interface PhotoDocument extends Photo, AnalysisResult {
  /** EXIF 元数据（相机参数、GPS 坐标等） */
  exif?: EXIFInfo;
  /** 国家名称（逆地理编码结果） */
  country?: string;
  /** 省/州名称 */
  province?: string;
  /** 城市名称 */
  city?: string;
  /** 区/县名称 */
  district?: string;
  /** 乡镇/街道名称 */
  township?: string;
  /** 商圈名称 */
  business_area?: string;
  /** 街道名称 */
  street?: string;
  /** 门牌号 */
  street_number?: string;
  /** 详细地址 */
  address?: string;
  /** 格式化的完整地址 */
  formatted_address?: string;
}

/**
 * 搜索请求接口
 *
 * 对应 POST /api/search 的请求体。
 * 支持关键词搜索和多种筛选条件的组合查询。
 */
export interface SearchRequest {
  /** 全文搜索关键词，匹配描述、标签和检测到的物体 */
  query?: string;
  /** 起始拍摄日期（格式 "YYYY-MM-DD"） */
  date_from?: string;
  /** 结束拍摄日期（格式 "YYYY-MM-DD"） */
  date_to?: string;
  /** 按标签过滤 */
  tags?: string[];
  /** 按检测到的物体过滤 */
  objects?: string[];
  /** 按场景类型过滤：indoor / outdoor / unknown */
  scene_type?: string;
  /** 按国家过滤 */
  country?: string;
  /** 按省份过滤 */
  province?: string;
  /** 按城市过滤 */
  city?: string;
  /** 按区/县过滤 */
  district?: string;
  /** 按处理状态过滤 */
  status?: string;
  /** 页码（从 1 开始） */
  page: number;
  /** 每页数量（最大 100） */
  page_size: number;
}

/**
 * 搜索响应接口
 *
 * 对应 POST /api/search 的响应体。
 * 包含分页后的照片列表和分页元数据。
 */
export interface SearchResponse {
  /** 匹配的照片文档列表 */
  hits: PhotoDocument[];
  /** 符合条件的照片总数 */
  total: number;
  /** 当前页码 */
  page: number;
  /** 每页数量 */
  page_size: number;
  /** 总页数 */
  total_pages: number;
}

/**
 * 统计信息响应接口
 *
 * 对应 GET /api/stats 的响应体。
 * 提供照片总数和按状态分组的统计数据。
 */
export interface StatsResponse {
  /** 照片总数 */
  total: number;
  /** 按处理状态分组的数量统计（键为状态名，值为数量） */
  by_status: Record<string, number>;
  /** 最近新增的照片数量（最近 24 小时内） */
  recent_count: number;
}

/**
 * 筛选选项响应接口
 *
 * 对应 GET /api/filters 的响应体。
 * 返回所有可用的筛选选项，用于前端搜索页面的下拉菜单。
 */
export interface FiltersResponse {
  /** 所有可用标签列表（按热度排序的前 50 个） */
  tags: string[];
  /** 场景类型列表 */
  scene_types: string[];
  /** 国家列表 */
  countries: string[];
  /** 省份列表 */
  provinces: string[];
  /** 城市列表 */
  cities: string[];
  /** 区/县列表 */
  districts: string[];
  /** 状态列表 */
  statuses: string[];
}

/**
 * EXIF 元数据信息接口
 *
 * 包含照片嵌入的 EXIF（Exchangeable Image File Format）元数据。
 * 所有字段均为可选，因为不同设备和格式支持的 EXIF 字段不同。
 * 地理位置信息（GPS）用于后续逆地理编码查询。
 *
 * @property date_time_original - 照片原始拍摄时间（ISO 8601 格式字符串）
 * @property camera_model - 相机/手机型号
 * @property lens_model - 镜头型号描述
 * @property focal_length - 焦距（如 "6.9mm"）
 * @property aperture - 光圈值（如 "f/1.8"）
 * @property shutter_speed - 快门速度（如 "1/125s"）
 * @property iso - ISO 感光度
 * @property gps_lat - GPS 纬度（WGS84 坐标系，十进制）
 * @property gps_lon - GPS 经度（WGS84 坐标系，十进制）
 */
export interface EXIFInfo {
  /** 原始拍摄时间（ISO 8601 格式，如 "2024-01-01T10:30:00Z"） */
  date_time_original?: string;
  /** 相机/手机型号（如 "iPhone 15 Pro"、"Canon EOS R5"） */
  camera_model?: string;
  /** 镜头型号描述 */
  lens_model?: string;
  /** 焦距（如 "6.9mm"、"50mm"） */
  focal_length?: string;
  /** 光圈值（如 "f/1.8"、"f/2.8"） */
  aperture?: string;
  /** 快门速度（如 "1/125s"、"1/1000s"） */
  shutter_speed?: string;
  /** ISO 感光度（如 100、400、3200） */
  iso?: number;
  /** GPS 纬度（WGS84 坐标系，十进制格式，北正南负） */
  gps_lat?: number;
  /** GPS 经度（WGS84 坐标系，十进制格式，东正西负） */
  gps_lon?: number;
}

/**
 * 颜色信息接口
 *
 * 由 AI 分析提取的照片主要颜色信息。
 * 包含颜色的中文名称和对应的十六进制色值，前端直接使用 hex 值进行 UI 渲染。
 *
 * @property name - 颜色中文名称（如 "天空蓝"、"草地绿"）
 * @property hex - 十六进制颜色值（如 "#87CEEB"），直接用于 CSS backgroundColor
 */
export interface ColorInfo {
  /** 颜色名称（中文，如 "天空蓝"、"夕阳橙"） */
  name: string;
  /** 十六进制颜色值（如 "#87CEEB"），前端直接用于背景色渲染 */
  hex: string;
}

/**
 * AI 分析结果接口
 *
 * 多模态 LLM 对照片内容分析后返回的结构化结果。
 * 包含图片的内容描述、标签、检测到的物体、场景类型、主要颜色等信息。
 * 后端要求 LLM 返回 JSON 格式响应，本接口映射该响应的数据结构。
 *
 * @property description - 自然语言描述（如"一张在公园里拍摄的照片，阳光明媚"）
 * @property tags - 标签列表，用于搜索和分类
 * @property objects - 检测到的物体列表
 * @property scene_type - 场景类型（indoor/outdoor/unknown）
 * @property colors - 主要颜色列表
 * @property people_count - 检测到的人数
 * @property has_text - 是否包含文字内容（如路牌、书籍等）
 * @property text - 识别到的文字内容（当 has_text 为 true 时）
 * @property confidence - AI 分析置信度（0-1 之间，可选）
 */
export interface AnalysisResult {
  /** 自然语言描述（由 LLM 生成，如"一张在公园里拍摄的照片，阳光明媚"） */
  description: string;
  /** 标签列表，用于搜索和分类（如 ["旅行", "风景", "海滩"]） */
  tags: string[];
  /** 检测到的物体列表（如 ["人", "沙滩", "大海", "云"]） */
  objects: string[];
  /** 场景类型：'indoor'（室内）、'outdoor'（室外）、'unknown'（未知） */
  scene_type: string;
  /** 主要颜色分析结果列表 */
  colors: ColorInfo[];
  /** AI 检测到的人数 */
  people_count: number;
  /** 是否包含可识别的文字内容（路标、书籍、菜单等） */
  has_text: boolean;
  /** 识别到的文字内容（当 has_text 为 true 时） */
  text: string;
  /** AI 分析置信度，取值范围 0-1，可选字段 */
  confidence?: number;
}

/**
 * 地理位置信息接口
 *
 * 通过逆地理编码服务（高德地图 API）从 GPS 坐标解析的地址信息。
 * 字段从省到街道逐级细化，formatted_address 为格式化的完整地址字符串。
 * 所有字段均为可选，因为并非所有照片都有 GPS 信息或解析成功。
 *
 * @property country - 国家名称
 * @property province - 省/自治区/直辖市
 * @property city - 城市名称
 * @property district - 区/县名称
 * @property address - 详细地址
 * @property formatted_address - 格式化的完整地址字符串
 */
export interface GeoInfo {
  /** 国家名称（如 "中国"） */
  country?: string;
  /** 省/自治区/直辖市名称（如 "北京市"、"广东省"） */
  province?: string;
  /** 城市名称（如 "北京"、"广州"） */
  city?: string;
  /** 区/县名称（如 "海淀区"、"天河区"） */
  district?: string;
  /** 详细地址信息 */
  address?: string;
  /** 格式化的完整地址字符串，由高德 API 返回 */
  formatted_address?: string;
}

/**
 * 完整照片文档接口
 *
 * 本接口是前端使用的最核心数据类型，通过多重继承合并了多类信息：
 * - Photo：基础照片信息（id、path、status 等）
 * - AnalysisResult：AI 分析结果（description、tags、colors 等）
 * - 额外字段：EXIF 元数据和地理位置信息
 *
 * 对应后端 Elasticsearch 索引中的文档结构，后端 API 返回的搜索结果
 * 直接映射为此类型。其中 country/province/city/district/address/formatted_address
 * 字段与 GeoInfo 接口重复，是为了在 Elasticsearch 索引中扁平化存储。
 *
 * @property exif - EXIF 元数据（可选，取决于照片是否包含 EXIF 信息）
 */
export interface PhotoDocument extends Photo, AnalysisResult {
  /** EXIF 元数据信息（相机型号、镜头、光圈、GPS 等），可选 */
  exif?: EXIFInfo;
  /** 国家名称（扁平化字段，与 GeoInfo.country 对应） */
  country?: string;
  /** 省/自治区/直辖市名称 */
  province?: string;
  /** 城市名称 */
  city?: string;
  /** 区/县名称 */
  district?: string;
  /** 详细地址 */
  address?: string;
  /** 格式化的完整地址字符串 */
  formatted_address?: string;
}

/**
 * 搜索请求参数接口
 *
 * 发送到 POST /api/search 的请求体数据结构。
 * 支持全文查询关键字搜索和多种筛选条件组合。
 * 后端使用 Elasticsearch 的 bool 查询构建复合搜索条件。
 * page 和 page_size 为必填参数，用于分页。
 *
 * @property query - 全文搜索关键词（匹配描述、标签和物体名称）
 * @property date_from - 拍摄日期范围起始（YYYY-MM-DD 格式）
 * @property date_to - 拍摄日期范围结束（YYYY-MM-DD 格式）
 * @property tags - 标签过滤列表（多选，文档需匹配所有标签）
 * @property objects - 检测到的物体过滤列表
 * @property scene_type - 场景类型过滤（indoor/outdoor/unknown）
 * @property country - 国家过滤
 * @property province - 省份过滤
 * @property city - 城市过滤
 * @property district - 区/县过滤
 * @property status - 处理状态过滤
 * @property page - 页码（从 1 开始，必填）
 * @property page_size - 每页数量（默认 20，最大 100，必填）
 */
export interface SearchRequest {
  /** 全文搜索关键词（匹配 description、tags、objects 字段） */
  query?: string;
  /** 拍摄日期范围起始（YYYY-MM-DD 格式，如 "2024-01-01"） */
  date_from?: string;
  /** 拍摄日期范围结束（YYYY-MM-DD 格式，如 "2024-12-31"） */
  date_to?: string;
  /** 标签过滤列表（如 ["旅行", "风景"]，文档需包含所有指定标签） */
  tags?: string[];
  /** 检测到的物体过滤列表（如 ["大海", "沙滩"]） */
  objects?: string[];
  /** 场景类型过滤：'indoor'（室内）、'outdoor'（室外）、'unknown'（未知） */
  scene_type?: string;
  /** 按国家过滤 */
  country?: string;
  /** 按省份过滤 */
  province?: string;
  /** 按城市过滤 */
  city?: string;
  /** 按区/县过滤 */
  district?: string;
  /** 按处理状态过滤 */
  status?: string;
  /** 页码（从 1 开始，必填） */
  page: number;
  /** 每页数量（默认 20，最大 100，必填） */
  page_size: number;
}

/**
 * 搜索响应数据接口
 *
 * POST /api/search 接口的响应数据结构。
 * hits 为匹配的照片文档列表，total 为总匹配数，用于分页计算。
 *
 * @property hits - 匹配的照片文档列表（当前页数据）
 * @property total - 总匹配数量（用于计算总页数）
 * @property page - 当前页码（从 1 开始）
 * @property page_size - 当前每页数量
 * @property total_pages - 总页数（根据 total 和 page_size 计算得出）
 */
export interface SearchResponse {
  /** 当前页的照片文档列表 */
  hits: PhotoDocument[];
  /** 总匹配数量，用于分页计算 */
  total: number;
  /** 当前页码（从 1 开始） */
  page: number;
  /** 每页显示数量 */
  page_size: number;
  /** 总页数（由后端根据 total/page_size 计算） */
  total_pages: number;
}

/**
 * 统计数据响应接口
 *
 * GET /api/stats 接口的响应数据结构。
 * 提供照片总数、各处理状态的分布统计和近期增长趋势。
 *
 * @property total - 照片总数
 * @property by_status - 按处理状态统计的照片数量映射（如 {"analyzed": 950, "failed": 20, ...}）
 * @property recent_count - 近期新增照片数量（具体时间范围由后端定义）
 */
export interface StatsResponse {
  /** 索引中的照片总数 */
  total: number;
  /** 按处理状态统计的照片数量，key 为状态名称，value 为数量 */
  by_status: Record<string, number>;
  /** 近期新增照片数量 */
  recent_count: number;
}

/**
 * 筛选选项响应接口
 *
 * GET /api/filters 接口的响应数据结构。
 * 返回所有可用的筛选选项列表，
 * 用于前端搜索页面的下拉筛选项，方便用户进行多条件组合筛选。
 *
 * @property tags - 所有可用标签列表（按热度排序的前 50 个）
 * @property scene_types - 场景类型列表
 * @property countries - 国家列表
 * @property provinces - 省份列表
 * @property cities - 城市列表
 * @property districts - 区/县列表
 * @property statuses - 状态列表
 */
export interface FiltersResponse {
  /** 所有可用标签列表（按热度排序的前 50 个） */
  tags: string[];
  /** 场景类型列表 */
  scene_types: string[];
  /** 国家列表 */
  countries: string[];
  /** 省份列表 */
  provinces: string[];
  /** 城市列表 */
  cities: string[];
  /** 区/县列表 */
  districts: string[];
  /** 状态列表 */
  statuses: string[];
}


