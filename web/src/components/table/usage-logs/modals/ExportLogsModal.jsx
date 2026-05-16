/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useMemo, useState } from 'react';
import { Modal, Button, Checkbox } from '@douyinfe/semi-ui';

function exportColumnLabel(t, key) {
  const labels = {
    time: t('时间'),
    channel: t('渠道'),
    username: t('用户'),
    token: t('令牌'),
    group: t('分组'),
    type: t('类型'),
    model: t('模型'),
    use_time: t('用时'),
    prompt: t('输入'),
    completion: t('输出'),
    cost: t('花费'),
    retry: t('重试'),
    ip: t('IP'),
    details: t('详情'),
  };
  return labels[key] || key;
}

const ExportLogsModal = ({
  showExportModal,
  setShowExportModal,
  visibleColumns,
  COLUMN_KEYS,
  isAdminUser,
  exportLoading,
  confirmLogExport,
  t,
}) => {
  const [localCols, setLocalCols] = useState({});

  const orderedKeys = useMemo(() => {
    const admin = [
      COLUMN_KEYS.TIME,
      COLUMN_KEYS.CHANNEL,
      COLUMN_KEYS.USERNAME,
      COLUMN_KEYS.TOKEN,
      COLUMN_KEYS.GROUP,
      COLUMN_KEYS.TYPE,
      COLUMN_KEYS.MODEL,
      COLUMN_KEYS.USE_TIME,
      COLUMN_KEYS.PROMPT,
      COLUMN_KEYS.COMPLETION,
      COLUMN_KEYS.COST,
      COLUMN_KEYS.RETRY,
      COLUMN_KEYS.IP,
      COLUMN_KEYS.DETAILS,
    ];
    const user = [
      COLUMN_KEYS.TIME,
      COLUMN_KEYS.TOKEN,
      COLUMN_KEYS.GROUP,
      COLUMN_KEYS.TYPE,
      COLUMN_KEYS.MODEL,
      COLUMN_KEYS.USE_TIME,
      COLUMN_KEYS.PROMPT,
      COLUMN_KEYS.COMPLETION,
      COLUMN_KEYS.COST,
      COLUMN_KEYS.IP,
      COLUMN_KEYS.DETAILS,
    ];
    return isAdminUser ? admin : user;
  }, [COLUMN_KEYS, isAdminUser]);

  useEffect(() => {
    if (showExportModal) {
      setLocalCols({ ...visibleColumns });
    }
  }, [showExportModal, visibleColumns]);

  const handleColumnChange = (columnKey, checked) => {
    setLocalCols((prev) => ({ ...prev, [columnKey]: checked }));
  };

  const handleSelectAllExport = (checked) => {
    const next = { ...localCols };
    orderedKeys.forEach((key) => {
      next[key] = checked;
    });
    setLocalCols(next);
  };

  const allChecked =
    orderedKeys.length > 0 &&
    orderedKeys.every((k) => !!localCols[k]);
  const someChecked = orderedKeys.some((k) => !!localCols[k]);

  const handleOk = async () => {
    const keys = orderedKeys.filter((k) => localCols[k]);
    await confirmLogExport(keys);
  };

  return (
    <Modal
      title={t('导出使用日志')}
      visible={showExportModal}
      onCancel={() => setShowExportModal(false)}
      confirmLoading={exportLoading}
      footer={
        <div className='flex justify-end gap-2'>
          <Button onClick={() => setShowExportModal(false)} disabled={exportLoading}>
            {t('取消')}
          </Button>
          <Button
            type='primary'
            loading={exportLoading}
            onClick={() => {
              handleOk().then(() => {});
            }}
          >
            {t('确定导出')}
          </Button>
        </div>
      }
    >
      <div style={{ marginBottom: 16 }}>
        <Checkbox
          checked={allChecked}
          indeterminate={someChecked && !allChecked}
          onChange={(e) => handleSelectAllExport(e.target.checked)}
        >
          {t('全选')}
        </Checkbox>
      </div>
      <div
        className='flex flex-wrap max-h-96 overflow-y-auto rounded-lg p-4'
        style={{ border: '1px solid var(--semi-color-border)' }}
      >
        {orderedKeys.map((columnKey) => (
          <div key={columnKey} className='w-1/2 mb-4 pr-2'>
            <Checkbox
              checked={!!localCols[columnKey]}
              onChange={(e) =>
                handleColumnChange(columnKey, e.target.checked)
              }
            >
              {exportColumnLabel(t, columnKey)}
            </Checkbox>
          </div>
        ))}
      </div>
    </Modal>
  );
};

export default ExportLogsModal;
