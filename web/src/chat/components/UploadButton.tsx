// ============================================
// 票据上传按钮 — ChatInput 的 prefix 插槽 (v5 多文件)
// 点击 → 选择图片 → uploadInvoice → 累加到文件列表
// 支持多次上传，预览列表中每张图片可单独移除
// ============================================

import { useState, useRef } from 'react';
import { Button, Image, Space, Typography, message, Spin } from 'antd';
import { PictureOutlined, DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { uploadInvoice } from '@/api';

const { Text } = Typography;
const MAX_SIZE = 10 * 1024 * 1024;
const MAX_FILES = 8;
const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export interface UploadedFile {
  path: string;
  url: string;
  name: string;
}

interface Props {
  value: UploadedFile[];
  onChange: (files: UploadedFile[]) => void;
  disabled?: boolean;
}

export function UploadButton({ value, onChange, disabled }: Props) {
  const [loading, setLoading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const handleSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (value.length >= MAX_FILES) {
      message.warning(`最多上传 ${MAX_FILES} 张票据`);
      return;
    }

    const allowed = ['image/jpeg', 'image/png', 'image/bmp', 'image/tiff', 'application/pdf'];
    if (!allowed.includes(file.type)) {
      message.warning('仅支持 JPG/PNG/PDF/BMP/TIFF 格式');
      return;
    }

    if (file.size > MAX_SIZE) {
      message.warning('图片大小不能超过 10MB');
      return;
    }

    setLoading(true);
    try {
      const result = await uploadInvoice(file);
      onChange([
        ...value,
        {
          path: result.file_path,
          url: result.url.startsWith('http') ? result.url : `${BASE_URL}${result.url}`,
          name: result.file_name,
        },
      ]);
      message.success('票据上传成功');
    } catch (err) {
      message.error(err instanceof Error ? err.message : '上传失败');
    } finally {
      setLoading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const handleRemove = (index: number) => {
    onChange(value.filter((_, i) => i !== index));
  };

  return (
    <div style={{ marginBottom: 6 }}>
      <input
        ref={fileRef}
        type="file"
        accept="image/jpeg,image/png,image/bmp,image/pdf"
        style={{ display: 'none' }}
        onChange={handleSelect}
      />

      {/* 已上传文件预览列表 */}
      {value.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 6 }}>
          {value.map((file, idx) => (
            <div key={`${file.path}-${idx}`} style={{ position: 'relative' }}>
              <Image
                src={file.url}
                width={48}
                height={48}
                style={{ objectFit: 'cover', borderRadius: 6, border: '1px solid #E8E8E8' }}
                preview={{ mask: '预览' }}
              />
              <Button
                type="text"
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => handleRemove(idx)}
                style={{
                  position: 'absolute',
                  top: -8,
                  right: -8,
                  padding: 0,
                  width: 18,
                  height: 18,
                  minWidth: 18,
                  borderRadius: '50%',
                  background: '#fff',
                  boxShadow: '0 0 4px rgba(0,0,0,0.15)',
                  fontSize: 10,
                }}
              />
            </div>
          ))}
        </div>
      )}

      {/* 上传按钮 */}
      <Button
        icon={loading ? <Spin size="small" /> : value.length > 0 ? <PlusOutlined /> : <PictureOutlined />}
        disabled={disabled || loading || value.length >= MAX_FILES}
        onClick={() => fileRef.current?.click()}
        size="small"
      >
        {loading ? '上传中...' : value.length > 0 ? `再传一张 (${value.length}/${MAX_FILES})` : '上传票据'}
      </Button>
    </div>
  );
}
