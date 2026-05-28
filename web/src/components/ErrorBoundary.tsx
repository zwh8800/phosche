import { Component, type ReactNode } from 'react';
import { Link } from 'react-router-dom';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  handleRefresh = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50 px-4 text-center">
          <svg
            className="mb-6 h-20 w-20 text-purple-300"
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
          <h1 className="text-2xl font-semibold text-gray-800">
            出错了
          </h1>
          <p className="mt-2 max-w-md text-sm text-gray-500">
            页面渲染时发生错误，请尝试刷新页面。
          </p>
          {this.state.error && (
            <p className="mt-3 max-w-lg rounded-lg bg-red-50 px-4 py-3 text-xs text-red-700 break-all">
              {this.state.error.message}
            </p>
          )}
          <div className="mt-8 flex items-center gap-3">
            <button
              type="button"
              onClick={this.handleRefresh}
              className="inline-flex items-center gap-2 rounded-lg bg-purple-600 px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-purple-700 cursor-pointer"
            >
              <svg
                className="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
              刷新页面
            </button>
            <Link
              to="/"
              className="rounded-lg bg-white px-5 py-2.5 text-sm font-medium text-gray-700 shadow-sm ring-1 ring-gray-200 transition-colors hover:bg-gray-50"
            >
              返回首页
            </Link>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
