/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import React, { useEffect, useState } from 'react';
import {
  Button,
  Card,
  Spin,
  Banner,
  Switch,
  Input,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, toBoolean } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

const initialKeys = [
  's3_image_setting.enabled',
  's3_image_setting.bucket',
  's3_image_setting.region',
  's3_image_setting.endpoint',
  's3_image_setting.access_key_id',
  's3_image_setting.secret',
  's3_image_setting.cdn',
  's3_image_setting.dir',
];

function emptyState() {
  const o = {};
  initialKeys.forEach((k) => {
    o[k] = k === 's3_image_setting.enabled' ? false : '';
  });
  return o;
}

const rowStyle = { marginBottom: 16 };
const labelStyle = { display: 'block', marginBottom: 6 };

const S3ImageSetting = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState(emptyState);
  const [originInputs, setOriginInputs] = useState(emptyState);
  const [loading, setLoading] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (!success) {
      showError(message);
      return;
    }
    const next = emptyState();
    data.forEach((item) => {
      if (!initialKeys.includes(item.key)) return;
      if (item.key === 's3_image_setting.enabled') {
        next[item.key] = toBoolean(item.value);
      } else {
        next[item.key] = item.value ?? '';
      }
    });
    setInputs(next);
    setOriginInputs(structuredClone(next));
  };

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        await getOptions();
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const onSave = async () => {
    const updates = [];
    for (const key of initialKeys) {
      const cur = inputs[key];
      const orig = originInputs[key];
      if (key === 's3_image_setting.secret' || key === 's3_image_setting.access_key_id') {
        if (cur !== '' && cur !== orig) {
          updates.push({ key, value: cur });
        }
        continue;
      }
      if (key === 's3_image_setting.enabled') {
        if (Boolean(cur) !== Boolean(orig)) {
          updates.push({ key, value: String(Boolean(cur)) });
        }
        continue;
      }
      if (cur !== orig) {
        updates.push({ key, value: String(cur ?? '') });
      }
    }
    if (updates.length === 0) {
      showError(t('你似乎并没有修改什么'));
      return;
    }
    const enabledUpd = updates.filter(
      (u) => u.key === 's3_image_setting.enabled',
    );
    const rest = updates.filter((u) => u.key !== 's3_image_setting.enabled');
    const ordered = [...rest, ...enabledUpd];
    setLoading(true);
    try {
      for (const u of ordered) {
        const res = await API.put('/api/option/', u);
        if (!res.data.success) {
          showError(res.data.message || t('保存失败'));
          return;
        }
      }
      showSuccess(t('保存成功'));
      await getOptions();
    } catch (e) {
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  };

  const setField = (key, v) => {
    setInputs((prev) => ({ ...prev, [key]: v }));
  };

  return (
    <Spin spinning={loading} size='large'>
      <Card style={{ marginTop: '10px' }}>
        <Banner
          type='info'
          description={t('s3_image_setting_help')}
          closeIcon={null}
          style={{ marginBottom: 16 }}
        />
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_enable')}
          </Text>
          <Switch
            checked={inputs['s3_image_setting.enabled']}
            onChange={(v) => setField('s3_image_setting.enabled', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_bucket')}
          </Text>
          <Input
            value={inputs['s3_image_setting.bucket']}
            onChange={(v) => setField('s3_image_setting.bucket', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_region')}
          </Text>
          <Input
            value={inputs['s3_image_setting.region']}
            onChange={(v) => setField('s3_image_setting.region', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_endpoint')}
          </Text>
          <Input
            value={inputs['s3_image_setting.endpoint']}
            onChange={(v) => setField('s3_image_setting.endpoint', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_cdn')}
          </Text>
          <Input
            value={inputs['s3_image_setting.cdn']}
            onChange={(v) => setField('s3_image_setting.cdn', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_dir')}
          </Text>
          <Input
            placeholder={t('s3_image_setting_dir_placeholder')}
            value={inputs['s3_image_setting.dir']}
            onChange={(v) => setField('s3_image_setting.dir', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_access_key_id')}
          </Text>
          <Input
            type='password'
            placeholder={t('s3_image_setting_secret_placeholder')}
            value={inputs['s3_image_setting.access_key_id']}
            onChange={(v) => setField('s3_image_setting.access_key_id', v)}
          />
        </div>
        <div style={rowStyle}>
          <Text strong style={labelStyle}>
            {t('s3_image_setting_secret')}
          </Text>
          <Input
            type='password'
            placeholder={t('s3_image_setting_secret_placeholder')}
            value={inputs['s3_image_setting.secret']}
            onChange={(v) => setField('s3_image_setting.secret', v)}
          />
        </div>
        <Button type='primary' onClick={onSave}>
          {t('保存')}
        </Button>
      </Card>
    </Spin>
  );
};

export default S3ImageSetting;
