/**
 * 文件发现 API——对接后端 `GET /v1/workspaces/{id}/files`。
 *
 * 后端接口不可用（未启动/路由缺失/权限不足）时优雅降级为空结果，绝不抛出，
 * 避免打断输入流程。
 */

import { get } from '../../api/transport'

export interface FileEntry {
  kind: 'file' | 'dir'
  /** 工作区相对路径。 */
  path: string
}

export interface FetchFilesOptions {
  /** 工作区 id（名称）。缺省使用 `main`。 */
  workspaceId?: string
  /** 可选子目录相对路径，后端以 `?path=` 下发该目录条目。 */
  path?: string
}

/** webui 缺少显式工作区概念时使用的默认工作区名（与后端默认示例一致）。 */
export const DEFAULT_WORKSPACE = 'main'

/**
 * 拉取工作区下的文件/目录条目。任何异常都返回空数组。
 */
export async function fetchWorkspaceFiles(
  options: FetchFilesOptions = {},
): Promise<FileEntry[]> {
  const name = options.workspaceId || DEFAULT_WORKSPACE
  const rel = options.path || ''
  const qs = rel ? `?path=${encodeURIComponent(rel)}` : ''
  try {
    const res = await get<{ files?: string[] }>(
      `/v1/workspaces/${encodeURIComponent(name)}/files${qs}`,
    )
    const list = res.files ?? []
    return list.map((p) => ({
      // 后端以 `[]string` 返回相对路径，目录末端通常带 '/'。
      kind: (p.endsWith('/') ? 'dir' : 'file') as FileEntry['kind'],
      path: p,
    }))
  } catch {
    // 优雅降级：不因端点缺失/网络错误阻断候选输入。
    return []
  }
}