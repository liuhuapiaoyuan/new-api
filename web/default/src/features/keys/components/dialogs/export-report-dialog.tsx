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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import dayjs from '@/lib/dayjs'
import { cn } from '@/lib/utils'

import { exportApiKeyUsageReport } from '../../api'
import {
  ERROR_MESSAGES,
  EXPORT_LOG_TYPE_OPTIONS,
  EXPORT_TIME_PRESETS,
  SUCCESS_MESSAGES,
  type ExportTimePresetId,
} from '../../constants'
import { useApiKeys } from '../api-keys-provider'

function toInputValue(date: Date): string {
  return dayjs(date).format('YYYY-MM-DDTHH:mm')
}

function fromInputValue(value: string): Date | null {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function getPresetRange(kind: ExportTimePresetId): { start: Date; end: Date } {
  const now = dayjs()
  switch (kind) {
    case 'today':
      return {
        start: now.startOf('day').toDate(),
        end: now.endOf('day').toDate(),
      }
    case '7d':
      return {
        start: now.subtract(6, 'day').startOf('day').toDate(),
        end: now.endOf('day').toDate(),
      }
    case '30d':
      return {
        start: now.subtract(29, 'day').startOf('day').toDate(),
        end: now.endOf('day').toDate(),
      }
    case 'month':
      return {
        start: now.startOf('month').toDate(),
        end: now.endOf('month').toDate(),
      }
  }
}

function getDefaultRange(): { start: Date; end: Date } {
  return getPresetRange('7d')
}

export function ExportReportDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useApiKeys()
  const isOpen = open === 'export'

  const [startInput, setStartInput] = useState('')
  const [endInput, setEndInput] = useState('')
  const [logType, setLogType] = useState('2')
  const [activePreset, setActivePreset] = useState<ExportTimePresetId | null>(
    '7d'
  )
  const [isExporting, setIsExporting] = useState(false)

  useEffect(() => {
    if (!isOpen) return
    const range = getDefaultRange()
    setStartInput(toInputValue(range.start))
    setEndInput(toInputValue(range.end))
    setLogType('2')
    setActivePreset('7d')
    setIsExporting(false)
  }, [isOpen, currentRow?.id])

  const applyPreset = (kind: ExportTimePresetId) => {
    const range = getPresetRange(kind)
    setStartInput(toInputValue(range.start))
    setEndInput(toInputValue(range.end))
    setActivePreset(kind)
  }

  const handleExport = async () => {
    if (!currentRow) return

    const start = fromInputValue(startInput)
    const end = fromInputValue(endInput)
    if (!start || !end) {
      toast.error(t(ERROR_MESSAGES.EXPORT_TIME_REQUIRED))
      return
    }
    if (start.getTime() > end.getTime()) {
      toast.error(t(ERROR_MESSAGES.EXPORT_TIME_INVALID))
      return
    }

    setIsExporting(true)
    try {
      const result = await exportApiKeyUsageReport(currentRow.id, {
        start_timestamp: Math.floor(start.getTime() / 1000),
        end_timestamp: Math.floor(end.getTime() / 1000),
        type: Number(logType) || 0,
      })
      if (result.success) {
        toast.success(t(SUCCESS_MESSAGES.EXPORT_STARTED))
        setOpen(null)
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.EXPORT_FAILED))
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.EXPORT_FAILED))
    } finally {
      setIsExporting(false)
    }
  }

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(nextOpen) => !nextOpen && setOpen(null)}
      title={t('Export Usage Report')}
      description={t('Download usage logs for API key {{name}}.', {
        name: currentRow?.name ?? '',
      })}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            disabled={isExporting}
            onClick={() => setOpen(null)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={isExporting || !currentRow}
            onClick={() => void handleExport()}
          >
            {isExporting ? t('Exporting...') : t('Export CSV')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Time Range')}</Label>
          <div className='flex flex-wrap gap-1.5'>
            {EXPORT_TIME_PRESETS.map((preset) => (
              <Button
                key={preset.id}
                type='button'
                size='sm'
                variant={activePreset === preset.id ? 'default' : 'secondary'}
                className='h-7 px-2.5 text-xs'
                disabled={isExporting}
                onClick={() => applyPreset(preset.id)}
              >
                {t(preset.labelKey)}
              </Button>
            ))}
          </div>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <div className='text-muted-foreground text-xs'>
                {t('Start Time')}
              </div>
              <Input
                type='datetime-local'
                value={startInput}
                disabled={isExporting}
                onChange={(e) => {
                  setStartInput(e.target.value)
                  setActivePreset(null)
                }}
                className='h-8 text-sm tabular-nums'
              />
            </div>
            <div className='space-y-1.5'>
              <div className='text-muted-foreground text-xs'>
                {t('End Time')}
              </div>
              <Input
                type='datetime-local'
                value={endInput}
                disabled={isExporting}
                onChange={(e) => {
                  setEndInput(e.target.value)
                  setActivePreset(null)
                }}
                className='h-8 text-sm tabular-nums'
              />
            </div>
          </div>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='export-log-type'>{t('Log Type')}</Label>
          <Select
            value={logType}
            onValueChange={(value) => {
              if (value != null) setLogType(String(value))
            }}
            disabled={isExporting}
          >
            <SelectTrigger
              id='export-log-type'
              className={cn('w-full')}
              size='sm'
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EXPORT_LOG_TYPE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.labelKey)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'Exports up to 500,000 rows. Narrow the time range if you need a smaller file.'
          )}
        </p>
      </div>
    </Dialog>
  )
}
