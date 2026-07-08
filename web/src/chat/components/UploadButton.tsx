// ============================================
// 票据上传按钮 —— ChatInput 的 prefix 插槽
// 点击 → 选择图片 → uploadInvoice → 预览缩略图
// ============================================

import { useState, useRef } from 'react';
import { Button, Image, Space, Typography, message, Spin } from 'antd';
import { PictureOutlined, DeleteOutlined } from '@ant-design/icons';
import { uploadInvoice } from '@/api';

const { Text } = Typography;
const MAX_SIZE = 10 * 1024 * 1024;
const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

interface UploadedFile {
  path: string;
  url: string;
  name: string;
}

interface Props {
  value: UploadedFile | null;
  onChange: (file: UploadedFile | null) => void;
  disabled?: boolean;
}

export function UploadButton({ value, onChange, disabled }: Props) {
  const [loading, setLoading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const handleSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    // 校验类型
    const allowed = ['image/jpeg', 'image/png', 'image/bmp', 'image/tiff', 'application/pdf'];
    if (!allowed.includes(file.type)) {
      message.warning('仅支持 JPG/PNG/PDF/BMP/TIFF 格式');
      return;
    }

    // 校验大小
    if (file.size > MAX_SIZE) {
      message.warning('图片大小不能超过 10MB');
      return;
    }

    setLoading(true);
    try {
      const result = await uploadInvoice(file);
      onChange({
        path: result.file_path,
        url: result.url.startsWith('http') ? result.url : `${BASE_URL}${result.url}`,
        name: result.file_name,
      });
      message.success('票据上传成功');
    } catch (err) {
      message.error(err instanceof Error ? err.message : '上传失败');
    } finally {
      setLoading(false);
      // 重置 input，允许重复选择同一文件
      if (fileRef.current) fileRef.current.value = '';
    }
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

      {!value ? (
        <Button
          icon={loading ? <Spin size="small" /> : <PictureOutlined />}
          disabled={disabled || loading}
          onClick={() => fileRef.current?.click()}
        >
          {loading ? '上传中...' : '上传票据'}
        </Button>
      ) : (
        <Space>
          <Image
            src={value.url}
            width={60}
            height={60}
            style={{ objectFit: 'cover', borderRadius: 6, border: '1px solid #E8E8E8' }}
            preview={{ mask: '预览' }}
          />
          <div>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {value.name}
            </Text>
            <br />
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              onClick={() => onChange(null)}
            >
              移除
            </Button>
          </div>
        </Space>
      )}
    </div>
  );
}
