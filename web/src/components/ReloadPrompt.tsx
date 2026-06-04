/**
 * @file PWA 更新提示组件。
 *
 * 当 Service Worker 检测到新版本时，显示更新提示横幅。
 * 用户点击"重新加载"后自动激活新版本并刷新页面。
 * 使用 vite-plugin-pwa 提供的 useRegisterSW hook 监听 SW 状态变化。
 */

import { useRegisterSW } from 'virtual:pwa-register/react'

/**
 * PWA 更新提示组件。
 *
 * 使用 vite-plugin-pwa 的 useRegisterSW hook 监听 Service Worker 状态变化，
 * 仅在检测到 offlineReady 或 needRefresh 状态时渲染提示横幅。
 *
 * 功能特性：
 * - 检测 Service Worker 更新（needRefresh）
 * - 检测离线就绪状态（offlineReady）
 * - 显示更新提示横幅（固定在页面右下角）
 * - 用户点击"重新加载"后调用 updateServiceWorker(true) 激活新版本并刷新页面
 * - 用户点击"关闭"可手动关闭提示
 *
 * @example
 * ```tsx
 * // 在 App.tsx 或 Layout 中使用
 * <ReloadPrompt />
 * ```
 *
 * @returns 更新提示横幅组件，无更新时返回 null
 */
export default function ReloadPrompt() {
  const {
    offlineReady: [offlineReady, setOfflineReady],
    needRefresh: [needRefresh, setNeedRefresh],
    updateServiceWorker,
  } = useRegisterSW({
    /** Service Worker 注册成功回调（当前为空实现） */
    onRegisteredSW() {},
    /** Service Worker 注册失败回调（当前为空实现） */
    onRegisterError() {},
  })

  /**
   * 关闭提示横幅。
   * 同时重置 offlineReady 和 needRefresh 状态。
   */
  const close = () => {
    setOfflineReady(false)
    setNeedRefresh(false)
  }

  // 当既非离线就绪也无需刷新时，不渲染任何内容
  if (!offlineReady && !needRefresh) return null

  return (
    <div className="fixed bottom-4 right-4 z-50 max-w-sm rounded-lg border border-gray-200 bg-white p-4 shadow-lg">
      <div className="mb-3 text-sm text-gray-700">
        {offlineReady ? (
          <span>应用已就绪，可离线使用</span>
        ) : (
          <span>发现新版本，点击重新加载以更新</span>
        )}
      </div>
      <div className="flex gap-2">
        {needRefresh && (
          <button
            onClick={() => updateServiceWorker(true)}
            className="rounded-md bg-purple-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-purple-700"
          >
            重新加载
          </button>
        )}
        <button
          onClick={close}
          className="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50"
        >
          关闭
        </button>
      </div>
    </div>
  )
}
