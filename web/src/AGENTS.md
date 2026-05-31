# AGENTS.md — web/src/

React SPA，照片时间线浏览 + 全文搜索。

## 快速命令

```bash
cd web && npm run dev       # Vite 开发服务器（代理 API 到 localhost:8080）
cd web && npm test          # vitest 单元测试
cd web && npx playwright test  # E2E（需后端运行）
```

## 路由

| 路径 | 页面 | 说明 |
|------|------|------|
| `/` | Timeline | 按日期分组，无限滚动 |
| `/search` | Search | 关键词 + 多条件筛选，无限滚动 |
| `/photo/*` | PhotoDetail | `*` 为文件路径，渲染 modal |

## 目录结构

```
├── App.tsx              # 路由 + QueryClient
├── types.ts             # TS 类型（Photo/EXIF/ColorInfo/SearchRequest 等）
├── api/
│   ├── client.ts        # axios 实例（baseURL: /api，10s 超时）
│   └── photos.ts        # fetchPhotos / searchPhotos / fetchPhotoDetail / fetchStats / fetchFilters
├── components/
│   ├── Layout.tsx       # 导航栏 + 移动端菜单
│   ├── PhotoDetail.tsx  # 详情弹窗（portal，EXIF/分析/文件信息）
│   └── ErrorBoundary.tsx
└── pages/
    ├── Timeline.tsx     # useInfiniteQuery + IntersectionObserver
    ├── Search.tsx       # useInfiniteQuery + useSearchParams 同步 URL
    ├── PhotoDetail.tsx  # 路由页，渲染 PhotoDetailModal
    └── NotFound.tsx
```

## 关键约定

- **照片 URL：** 缩略图 `/photos/{path}?thumb=1`，原图 `/photos/{path}?convert=1`（HEIC 转 JPEG）。路径 `path.replace(/^\/+/, '')` 去前导斜杠。
- **无限滚动：** `useInfiniteQuery` + `IntersectionObserver` 观察 sentinel div，阈值 0.1。
- **颜色：** 后端返回 `ColorInfo { name, hex }`，前端直接用 `c.hex` 做 `backgroundColor`，不做映射。
- **状态徽章：** `STATUS_LABELS` / `STATUS_COLORS` 在各页面各自定义，未抽取共享常量。
- **图片加载失败：** `onError` 隐藏 `<img>`，显示 "无法加载图片" 占位文字。
- **搜索 URL：** 用 `useSearchParams` 同步筛选条件到 URL，筛选变化 300ms 防抖。

## 反模式

- 不要手动管理缓存。React Query 通过 `queryKey` 自动处理。
- 不要硬编码照片 URL。用 `path.replace(/^\/+/, '')` 确保无前导斜杠。
- 不要遗漏 `?thumb=1`。列表页必须用缩略图，否则加载巨大原图。
- 不要修改 `api/client.ts` 的 10s 超时，这是调优值。
