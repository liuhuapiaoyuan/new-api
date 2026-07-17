/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import type {
  ApiKey,
  ApiResponse,
  GetApiKeysParams,
  GetApiKeysResponse,
  SearchApiKeysParams,
  ApiKeyFormData,
  ExportApiKeyUsageParams,
} from './types'

// ============================================================================
// API Key Management
// ============================================================================

// Get paginated API keys list
export async function getApiKeys(
  params: GetApiKeysParams = {}
): Promise<GetApiKeysResponse> {
  const { p = 1, size = 10 } = params
  const res = await api.get(`/api/token/?p=${p}&size=${size}`)
  return res.data
}

// Search API keys by keyword or token (with pagination)
export async function searchApiKeys(
  params: SearchApiKeysParams
): Promise<GetApiKeysResponse> {
  const { keyword = '', token = '', p, size } = params
  const queryParams = new URLSearchParams()
  if (keyword) queryParams.set('keyword', keyword)
  if (token) queryParams.set('token', token)
  if (p != null) queryParams.set('p', String(p))
  if (size != null) queryParams.set('size', String(size))
  const res = await api.get(`/api/token/search?${queryParams.toString()}`)
  return res.data
}

// Get single API key by ID
export async function getApiKey(id: number): Promise<ApiResponse<ApiKey>> {
  const res = await api.get(`/api/token/${id}`)
  return res.data
}

// Create a new API key
export async function createApiKey(
  data: ApiKeyFormData
): Promise<ApiResponse<ApiKey>> {
  const res = await api.post('/api/token/', data)
  return res.data
}

// Update an existing API key
export async function updateApiKey(
  data: ApiKeyFormData & { id: number }
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put('/api/token/', data)
  return res.data
}

// Delete a single API key
export async function deleteApiKey(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/token/${id}/`)
  return res.data
}

// Batch delete multiple API keys
export async function batchDeleteApiKeys(
  ids: number[]
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/token/batch', { ids })
  return res.data
}

// Update API key status (enable/disable)
export async function updateApiKeyStatus(
  id: number,
  status: number
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put('/api/token/?status_only=true', { id, status })
  return res.data
}

// Fetch the real (unmasked) key for a token by ID
export async function fetchTokenKey(
  id: number
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const res = await api.post(`/api/token/${id}/key`)
  return res.data
}

// Batch fetch real (unmasked) keys for multiple tokens
export async function fetchTokenKeysBatch(ids: number[]): Promise<{
  success: boolean
  message?: string
  data?: { keys: Record<number, string> }
}> {
  const res = await api.post('/api/token/batch/keys', { ids })
  return res.data
}

function parseFilenameFromDisposition(header: string | undefined): string | null {
  if (!header) return null
  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(header)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1].trim())
    } catch {
      return utf8Match[1].trim()
    }
  }
  const plainMatch = /filename="?([^";]+)"?/i.exec(header)
  return plainMatch?.[1]?.trim() || null
}

async function readBlobErrorMessage(blob: Blob): Promise<string | null> {
  try {
    const text = await blob.text()
    const json = JSON.parse(text) as { success?: boolean; message?: string }
    if (json && typeof json.message === 'string' && json.message) {
      return json.message
    }
  } catch {
    /* not JSON */
  }
  return null
}

async function detectExportError(
  blob: Blob,
  contentType: string
): Promise<string | null> {
  if (
    contentType.includes('application/json') ||
    contentType.includes('text/json') ||
    blob.type === 'application/json'
  ) {
    return (await readBlobErrorMessage(blob)) || 'Export failed'
  }

  // Backend error JSON may arrive without a JSON content-type through proxies.
  try {
    const peek = await blob.slice(0, 16).text()
    const trimmed = peek.replace(/^\uFEFF/, '').trimStart()
    if (trimmed.startsWith('{')) {
      return readBlobErrorMessage(blob)
    }
  } catch {
    /* ignore peek failures */
  }
  return null
}

/** Download a CSV usage report for one API key within a time range. */
export async function exportApiKeyUsageReport(
  id: number,
  params: ExportApiKeyUsageParams
): Promise<{ success: true } | { success: false; message: string }> {
  try {
    const res = await api.get(`/api/token/${id}/export`, {
      params,
      responseType: 'blob',
      skipBusinessError: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    })

    const blob = res.data as Blob
    const contentType = String(res.headers['content-type'] || '')
    const errorMessage = await detectExportError(blob, contentType)
    if (errorMessage) {
      return { success: false, message: errorMessage }
    }

    const filename =
      parseFilenameFromDisposition(
        res.headers['content-disposition'] as string | undefined
      ) || `token-${id}-usage.csv`

    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    anchor.rel = 'noopener'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)

    return { success: true }
  } catch (error: unknown) {
    const axiosError = error as {
      response?: { data?: Blob; status?: number }
      message?: string
    }
    if (axiosError.response?.data instanceof Blob) {
      const message = await readBlobErrorMessage(axiosError.response.data)
      if (message) return { success: false, message }
    }
    return {
      success: false,
      message: axiosError.message || 'Export failed',
    }
  }
}
